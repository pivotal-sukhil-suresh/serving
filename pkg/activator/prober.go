package activator

import (
	"errors"
	"fmt"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"net/http"
	"net/url"
	"go.uber.org/zap"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"time"
)

type Prober interface {
	CheckProbe(probe *v1.Probe, logger *zap.SugaredLogger) (ready bool, err error)
}

type HttpGetProber struct{}

type TCPSocketProber struct{}

func CheckProbe(revision *v1alpha1.Revision, endpoint Endpoint, logger *zap.SugaredLogger) {

	// FIXME: handle case when revision.Spec.Container.ReadinessProbe is nil
	probe := revision.Spec.Container.ReadinessProbe.DeepCopy()
	probe.HTTPGet.Host = endpoint.FQDN
	probe.HTTPGet.Port.Type = intstr.Int
	probe.HTTPGet.Port.IntVal = endpoint.Port

	// FIXME: handle default values not being set
	probe.HTTPGet.Scheme = "http"

	// FIXME: For now, assume UserContainer readiness HTTPGetProbe is specified
	maxRetry := 60
	i := 1
	for i = 1; i < maxRetry; i++ {
		ready, err := HttpGetProber{}.CheckProbe(probe, logger)
		if err !=  nil {
			logger.Errorf("error while checking probe: %#v", err)
			break
		}

		if ready {
			break
		} else {
			time.Sleep(time.Second * 1)
		}
	}
	logger.Infof("took %d probe retries for readiness of endpoint %#v", i, endpoint)
}

func (p HttpGetProber) CheckProbe(probe *v1.Probe, logger *zap.SugaredLogger) (ready bool, err error) {
	if probe == nil {
		return false, errors.New("probe cannot be nil")
	}

	host, err := p.getHostFromProbe(probe)
	if err != nil {
		return false, err
	}

	url := url.URL{
		Host:   host,
		Scheme: string(probe.HTTPGet.Scheme),
		Path:   probe.HTTPGet.Path,
	}
	logger.Infof("checking probe url: %s", url.String())

	res, err := http.Get(url.String())
	if err != nil {
		return false, err
	}

	return res.StatusCode == http.StatusOK, nil
}

func (p TCPSocketProber) CheckProbe(probe *v1.Probe) (ready bool, err error) {
	if probe == nil {
		return false, errors.New("probe cannot be nil")
	}

	host, err := p.getHostFromProbe(probe)
	if err != nil {
		return false, err
	}

	conn, err := net.Dial("tcp", host)
	if err != nil {
		return false, err
	}

	conn.Close()

	return true, nil
}

func (HttpGetProber) getHostFromProbe(probe *v1.Probe) (host string, err error) {
	switch probe.HTTPGet.Port.Type {
	case intstr.Int:
		host = fmt.Sprintf("%s:%d", probe.HTTPGet.Host, probe.HTTPGet.Port.IntVal)
	case intstr.String:
		host = fmt.Sprintf("%s:%s", probe.HTTPGet.Host, probe.HTTPGet.Port.StrVal)
	default:
		err = errors.New(fmt.Sprintf("unsupported port type %d", probe.HTTPGet.Port.Type))
	}
	return host, err
}

func (TCPSocketProber) getHostFromProbe(probe *v1.Probe) (host string, err error) {
	switch probe.TCPSocket.Port.Type {
	case intstr.Int:
		host = fmt.Sprintf("%s:%d", probe.TCPSocket.Host, probe.TCPSocket.Port.IntVal)
	case intstr.String:
		host = fmt.Sprintf("%s:%s", probe.TCPSocket.Host, probe.TCPSocket.Port.StrVal)
	default:
		err = errors.New(fmt.Sprintf("unsupported port type %d", probe.TCPSocket.Port.Type))
	}
	return host, err
}
