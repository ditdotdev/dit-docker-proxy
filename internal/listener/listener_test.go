/*
 * Copyright Datadatdat.
 */

package listener

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/datadatdat/datadatdat-docker-proxy/internal/forwarder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// errorReader is a reader that always returns an error, used to test ReadAll failure paths.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

type MockForwarder struct {
	mock.Mock
}

func (f *MockForwarder) CreateVolume(request forwarder.CreateVolumeRequest) forwarder.VolumeResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.VolumeResponse)
}

func (f *MockForwarder) GetPath(request forwarder.VolumeRequest) forwarder.GetPathResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.GetPathResponse)
}

func (f *MockForwarder) GetVolume(request forwarder.VolumeRequest) forwarder.GetVolumeResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.GetVolumeResponse)
}

func (f *MockForwarder) ListVolumes() forwarder.ListVolumeResponse {
	args := f.Called()
	return args.Get(0).(forwarder.ListVolumeResponse)
}

func (f *MockForwarder) MountVolume(request forwarder.MountVolumeRequest) forwarder.GetPathResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.GetPathResponse)
}

func (f *MockForwarder) PluginActivate() forwarder.PluginDescription {
	args := f.Called()
	return args.Get(0).(forwarder.PluginDescription)
}

func (f *MockForwarder) RemoveVolume(request forwarder.VolumeRequest) forwarder.VolumeResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.VolumeResponse)
}

func (f *MockForwarder) VolumeCapabilities() forwarder.VolumeCapabilities {
	args := f.Called()
	return args.Get(0).(forwarder.VolumeCapabilities)
}

func (f *MockForwarder) UnmountVolume(request forwarder.MountVolumeRequest) forwarder.VolumeResponse {
	args := f.Called(request)
	return args.Get(0).(forwarder.VolumeResponse)
}

func TestCreateVolume(t *testing.T) {
	f := new(MockForwarder)
	f.On("CreateVolume", mock.Anything).Return(forwarder.VolumeResponse{})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\",\"Opts\":{}}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Create", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestGetVolume(t *testing.T) {
	f := new(MockForwarder)
	f.On("GetVolume", mock.Anything).Return(forwarder.GetVolumeResponse{
		Volume: forwarder.Volume{
			Name:       "foo/vol",
			Mountpoint: "/vol",
			Status:     map[string]string{},
		},
	})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\"}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Get", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\",\"Volume\":{\"Name\":\"foo/vol\",\"Mountpoint\":\"/vol\",\"Status\":{}}}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestGetPath(t *testing.T) {
	f := new(MockForwarder)
	f.On("GetPath", mock.Anything).Return(forwarder.GetPathResponse{Mountpoint: "/vol"})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\"}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Path", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\",\"Mountpoint\":\"/vol\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestListVolumes(t *testing.T) {
	f := new(MockForwarder)
	f.On("ListVolumes").Return(forwarder.ListVolumeResponse{
		Volumes: []forwarder.Volume{
			{
				Name:       "foo/vol",
				Mountpoint: "/foo/vol",
				Status:     map[string]string{},
			},
		},
	})
	l := create(f, "/socket")
	req, _ := http.NewRequest("POST", "/VolumeDriver.List", nil)
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\",\"Volumes\":[{\"Name\":\"foo/vol\",\"Mountpoint\":\"/foo/vol\",\"Status\":{}}]}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestMountVolume(t *testing.T) {
	f := new(MockForwarder)
	f.On("MountVolume", mock.Anything).Return(forwarder.GetPathResponse{Mountpoint: "/vol"})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\",\"ID\":\"0\"}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Mount", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\",\"Mountpoint\":\"/vol\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestPluginActivate(t *testing.T) {
	f := new(MockForwarder)
	f.On("PluginActivate").Return(forwarder.PluginDescription{Implements: []string{"VolumeDriver"}})
	l := create(f, "/socket")
	req, _ := http.NewRequest("POST", "/Plugin.Activate", nil)
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Implements\":[\"VolumeDriver\"]}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestRemoveVolume(t *testing.T) {
	f := new(MockForwarder)
	f.On("RemoveVolume", mock.Anything).Return(forwarder.VolumeResponse{})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\"}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Remove", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestVolumeCapabilities(t *testing.T) {
	f := new(MockForwarder)
	f.On("VolumeCapabilities").Return(forwarder.VolumeCapabilities{
		Capabilities: forwarder.Capability{Scope: "local"},
	})
	l := create(f, "/socket")
	req, _ := http.NewRequest("POST", "/VolumeDriver.Capabilities", nil)
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Capabilities\":{\"Scope\":\"local\"}}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestUnmountVolume(t *testing.T) {
	f := new(MockForwarder)
	f.On("UnmountVolume", mock.Anything).Return(forwarder.VolumeResponse{})
	l := create(f, "/socket")
	body := "{\"Name\":\"foo/vol\",\"ID\":\"0\"}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Unmount", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestNew(t *testing.T) {
	f := new(MockForwarder)
	l := New(f, "/tmp/test.sock")
	assert.NotNil(t, l)
}

func TestSetLogging(t *testing.T) {
	f := new(MockForwarder)
	l := New(f, "/tmp/test.sock")
	// SetLogging should not panic and should toggle the log field
	l.SetLogging(true)
	l.SetLogging(false)
}

func TestListenErrorBadPath(t *testing.T) {
	f := new(MockForwarder)
	// Use a path that is guaranteed to fail on any OS
	l := New(f, "/nonexistent/deeply/nested/dir/test.sock")
	err := l.Listen()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listen failed on")
}

func TestListenSuccessAndServe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not reliably available on Windows")
	}

	f := new(MockForwarder)
	f.On("PluginActivate").Return(forwarder.PluginDescription{Implements: []string{"VolumeDriver"}})

	sockDir, err := os.MkdirTemp("", "listener-test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sockDir) }()

	sockPath := filepath.Join(sockDir, "test.sock")
	l := New(f, sockPath)

	// Run Listen in a goroutine since it blocks
	go func() {
		_ = l.Listen()
	}()

	// Wait for the socket file to appear, confirming Listen() bound successfully
	assert.Eventually(t, func() bool {
		_, statErr := os.Stat(sockPath)
		return statErr == nil
	}, 2*1e9, 50*1e6, "socket file did not appear")
}

func TestServeHTTPWithLoggingEnabled(t *testing.T) {
	f := new(MockForwarder)
	f.On("CreateVolume", mock.Anything).Return(forwarder.VolumeResponse{})
	l := create(f, "/socket")
	l.log = true

	body := "{\"Name\":\"foo/vol\",\"Opts\":{}}"
	req, _ := http.NewRequest("POST", "/VolumeDriver.Create", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\"}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestServeHTTPNoBodyWithLoggingEnabled(t *testing.T) {
	f := new(MockForwarder)
	f.On("ListVolumes").Return(forwarder.ListVolumeResponse{
		Volumes: []forwarder.Volume{},
	})
	l := create(f, "/socket")
	l.log = true

	req, _ := http.NewRequest("POST", "/VolumeDriver.List", nil)
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, "{\"Err\":\"\",\"Volumes\":[]}", rr.Body.String())
	f.AssertExpectations(t)
}

func TestServeHTTPReadAllError(t *testing.T) {
	f := new(MockForwarder)
	// The forwarder is still called even when ReadAll fails, because the current code
	// unconditionally calls funcValue.Call after setting err. The response is then
	// overridden by the error path since err != nil.
	f.On("CreateVolume", mock.Anything).Return(forwarder.VolumeResponse{})
	l := create(f, "/socket")

	req, _ := http.NewRequest("POST", "/VolumeDriver.Create", io.NopCloser(&errorReader{}))
	rr := httptest.NewRecorder()
	handler, _ := l.mux.Handler(req)
	handler.ServeHTTP(rr, req)
	assert.Contains(t, rr.Body.String(), "simulated read error")
}
