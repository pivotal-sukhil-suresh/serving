package activator

import (
	"fmt"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type Matcher int

const (
	Exact    Matcher = 0
	Contains Matcher = 1
)

type testData struct {
	probe        *v1.Probe
	want         result
	errorMatcher Matcher
}

type result struct {
	ready bool
	err   error
}

func (r result) String() string {
	var errorString string
	if r.err != nil {
		errorString = r.err.Error()
	}
	return fmt.Sprintf("result(ready: %t, err: %s)", r.ready, errorString)
}

func (r result) Match(other result, matcher Matcher) bool {
	if r.ready != other.ready {
		return false
	}
	if (r.err == nil) && (other.err != nil) {
		return false
	}
	if (r.err != nil) && (other.err == nil) {
		return false
	}
	if r.err == nil && other.err == nil {
		return true
	}

	switch matcher {
	case Exact:
		return r.err.Error() == other.err.Error()
	case Contains:
		return strings.Contains(r.err.Error(), other.err.Error())
	default:
		return false
	}
}

func TestCheckHttpGetReadiness(t *testing.T) {
	server := getTestHttpServer(t)
	defer server.Close()

	url, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("error parsing test server url(%s): %s", server.URL, err.Error())
	}

	testCases := generateHttpGetTestCases(t, url)
	for testName, testData := range testCases {
		ready, err := HttpGetProber{}.CheckProbe(testData.probe)
		got := result{ready, err}

		if !got.Match(testData.want, testData.errorMatcher) {
			t.Fatalf("%s. want: %v. got: %v", testName, testData.want, got)
		}
	}
}

func generateHttpGetTestCases(t *testing.T, url *url.URL) (testCases map[string]testData) {
	testCases = make(map[string]testData)

	error := errors.New("probe cannot be nil")
	testCases["probe is nil"] = testData{nil, result{false, error}, Exact}

	probe := getTestHttpGetProbe(t, url, intstr.String)
	testCases["probe with port as string"] = testData{probe, result{true, nil}, Exact}

	probe = getTestHttpGetProbe(t, url, intstr.Int)
	testCases["probe with port as integer"] = testData{probe, result{true, nil}, Exact}

	badProbe := probe.DeepCopy()
	badProbe.HTTPGet.Host = "bad_host_name"
	error = errors.New("no such host")
	testCases["probe with bad host name"] = testData{badProbe, result{false, error}, Contains}

	badProbe = probe.DeepCopy()
	badProbe.HTTPGet.Path = "bad_host_path"
	testCases["probe with bad host path"] = testData{badProbe, result{false, nil}, Exact}

	badProbe = probe.DeepCopy()
	badProbe.HTTPGet.Port = intstr.IntOrString{Type: 3}
	error = errors.New(fmt.Sprintf("unsupported port type %d", badProbe.HTTPGet.Port.Type))
	testCases["probe with unsupported port type"] = testData{badProbe, result{false, error}, Exact}

	return testCases
}

func getTestHttpServer(t *testing.T) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return httptest.NewServer(handler)
}

func getTestHttpGetProbe(t *testing.T, url *url.URL, portType intstr.Type) *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Host:   url.Hostname(),
				Path:   "/health",
				Scheme: v1.URISchemeHTTP,
				Port:   getProbePort(t, url, portType),
			},
		},
	}
}

func TestCheckTCPSocketReadiness(t *testing.T) {
	listener := getTestSocketListener(t)
	defer listener.Close()

	url := &url.URL{Host: listener.Addr().String()}

	testCases := generateTCPSocketTestCases(t, url)
	for testName, testData := range testCases {
		ready, err := TCPSocketProber{}.CheckProbe(testData.probe)
		got := result{ready, err}

		if !got.Match(testData.want, testData.errorMatcher) {
			t.Fatalf("%s. want: %v. got: %v", testName, testData.want, got)
		}
	}
}

func generateTCPSocketTestCases(t *testing.T, url *url.URL) (testCases map[string]testData) {
	testCases = make(map[string]testData)

	error := errors.New("probe cannot be nil")
	testCases["probe is nil"] = testData{nil, result{false, error}, Exact}

	probe := getTestTCPSocketProbe(t, url, intstr.String)
	testCases["probe with port as string"] = testData{probe, result{true, nil}, Exact}

	probe = getTestTCPSocketProbe(t, url, intstr.Int)
	testCases["probe with port as integer"] = testData{probe, result{true, nil}, Exact}

	badProbe := probe.DeepCopy()
	badProbe.TCPSocket.Port.IntVal = 1
	error = errors.New("connection refused")
	testCases["probe with bad port number"] = testData{badProbe, result{false, error}, Contains}

	badProbe = probe.DeepCopy()
	badProbe.TCPSocket.Host = "bad_host_name"
	error = errors.New("no such host")
	testCases["probe with bad host name"] = testData{badProbe, result{false, error}, Contains}

	badProbe = probe.DeepCopy()
	badProbe.TCPSocket.Port = intstr.IntOrString{Type: 3}
	error = errors.New(fmt.Sprintf("unsupported port type %d", badProbe.TCPSocket.Port.Type))
	testCases["probe with unsupported port type"] = testData{badProbe, result{false, error}, Exact}

	return testCases
}

func getTestSocketListener(t *testing.T) net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("error creating test tcp socket listener: %s", err.Error())
	}

	go func() {
		_, err = listener.Accept()
		//if err != nil {
		//	t.Fatalf("error accepting connections on test tcp socket listener: %s", err.Error())
		//}
	}()

	return listener
}

func getTestTCPSocketProbe(t *testing.T, url *url.URL, portType intstr.Type) *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			TCPSocket: &v1.TCPSocketAction{
				Host: url.Hostname(),
				Port: getProbePort(t, url, portType),
			},
		},
	}
}

func getProbePort(t *testing.T, url *url.URL, portType intstr.Type) (probePort intstr.IntOrString) {
	switch portType {
	case intstr.String:
		probePort = intstr.IntOrString{
			Type:   intstr.String,
			StrVal: url.Port(),
		}
	case intstr.Int:
		urlPort, err := strconv.ParseInt(url.Port(), 10, 32)
		if err != nil {
			t.Fatalf("error parsing port(%s) : %s", url.Port(), err.Error())
		}
		probePort = intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: int32(urlPort),
		}
	default:
		t.Fatalf("unsupported probe port type: %v", portType)
	}
	return probePort
}
