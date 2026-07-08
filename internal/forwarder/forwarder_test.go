// Copyright Dit 2026
// SPDX-License-Identifier: BUSL-1.1

package forwarder

import (
	"context"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testForwarder(handler http.Handler) (Forwarder, func()) {
	s := httptest.NewServer(handler)

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
		},
	}

	return NewClient(cli), s.Close
}

func TestPluginActivate(t *testing.T) {
	f := New("localhost", 5001)
	resp := f.PluginActivate()
	assert.Equal(t, resp.Implements[0], "VolumeDriver")
}

func TestVolumeDriverCapabilities(t *testing.T) {
	f := New("localhost", 5001)
	resp := f.VolumeCapabilities()
	assert.Equal(t, resp.Capabilities.Scope, "local")
}

func TestListVolumes(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.RequestURI == "/v1/repositories" {
			_, _ = w.Write([]byte("[{\"name\":\"foo\",\"properties\":{}}]"))
		} else {
			assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes")
			_, _ = w.Write([]byte("[{\"name\":\"v0\",\"properties\":{},\"config\":{\"mountpoint\":\"/v0\"}}," +
				"{\"name\":\"v1\",\"properties\":{},\"config\":{\"mountpoint\":\"/v1\"}}]"))
		}
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.ListVolumes()
	if assert.Empty(t, resp.Err) &&
		assert.Equal(t, len(resp.Volumes), 2) {
		assert.Equal(t, resp.Volumes[0].Name, "foo_v0")
		assert.Equal(t, resp.Volumes[0].Mountpoint, "/v0")
		assert.Equal(t, len(resp.Volumes[0].Status), 0)
		assert.Equal(t, resp.Volumes[1].Name, "foo_v1")
		assert.Equal(t, resp.Volumes[1].Mountpoint, "/v1")
		assert.Equal(t, len(resp.Volumes[1].Status), 0)
	}
}

func TestListVolumesRepoError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such repository\"}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.ListVolumes()
	assert.Equal(t, resp.Err, "no such repository")
}

func TestListVolumesVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.RequestURI == "/v1/repositories" {
			_, _ = w.Write([]byte("[{\"name\":\"foo\",\"properties\":{}}]"))
		} else {
			assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("{\"message\":\"no such volume\"}"))
		}
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.ListVolumes()
	assert.Equal(t, resp.Err, "no such volume")
}

func TestGetVolume(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		_, _ = w.Write([]byte("{\"name\":\"vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/vol\"}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetVolume(VolumeRequest{Name: "foo/vol"})
	if assert.Empty(t, resp.Err) {
		assert.Equal(t, resp.Volume.Name, "foo_vol")
		assert.Equal(t, resp.Volume.Mountpoint, "/vol")
	}
}

func TestGetVolumeUnderscoreFormat(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		_, _ = w.Write([]byte("{\"name\":\"vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/vol\"}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetVolume(VolumeRequest{Name: "foo_vol"})
	if assert.Empty(t, resp.Err) {
		assert.Equal(t, resp.Volume.Name, "foo_vol")
		assert.Equal(t, resp.Volume.Mountpoint, "/vol")
	}
}

func TestGetVolumeBadName(t *testing.T) {
	f := New("localhost", 5001)

	resp := f.GetVolume(VolumeRequest{Name: "foo"})
	assert.Equal(t, resp.Err, "volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

// Volume names with additional underscores after the separator (e.g. created
// by upstream tooling that uses underscores inside the volume name itself)
// must split on the FIRST underscore and treat the rest as the volume name.
// Pre-fix, the regex used `[^_]+` on both groups and these names returned
// "volume name must be of the form..." instead.
func TestGetVolumeNameWithMultipleUnderscores(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/repo/volumes/my_vol")
		_, _ = w.Write([]byte("{\"name\":\"my_vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/my_vol\"}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetVolume(VolumeRequest{Name: "repo_my_vol"})
	if assert.Empty(t, resp.Err) {
		assert.Equal(t, resp.Volume.Name, "repo_my_vol")
		assert.Equal(t, resp.Volume.Mountpoint, "/my_vol")
	}
}

// parseVolumeName direct test for the multi-separator case in the
// back-compat slash form. The slash form encodes the volume-name slash
// as %2F when the URL is built; we don't assert on the wire-level
// encoding here, just that the split itself is correct.
func TestParseVolumeNameSlashWithExtraSegments(t *testing.T) {
	repo, vol, err := parseVolumeName("repo/sub/vol")
	assert.NoError(t, err)
	assert.Equal(t, "repo", repo)
	assert.Equal(t, "sub/vol", vol)
}

func TestParseVolumeNameUnderscoreWithExtraSegments(t *testing.T) {
	repo, vol, err := parseVolumeName("repo_sub_vol")
	assert.NoError(t, err)
	assert.Equal(t, "repo", repo)
	assert.Equal(t, "sub_vol", vol)
}

func TestParseVolumeNameNoSeparator(t *testing.T) {
	_, _, err := parseVolumeName("just-a-name")
	assert.Error(t, err)
}

func TestGetVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such volume\"}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetVolume(VolumeRequest{Name: "foo/vol"})
	assert.Equal(t, resp.Err, "no such volume")
}

func TestGetPath(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		_, _ = w.Write([]byte("{\"name\":\"vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/vol\"}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetPath(VolumeRequest{Name: "foo/vol"})
	if assert.Empty(t, resp.Err) {
		assert.Equal(t, resp.Mountpoint, "/vol")
	}
}

func TestGetPathError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such volume\"}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.GetPath(VolumeRequest{Name: "foo/vol"})
	assert.Equal(t, resp.Err, "no such volume")
}

func TestCreateVolume(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes")
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, string(body), "{\"name\":\"vol\",\"properties\":{\"a\":\"b\"}}\n")
		_, _ = w.Write([]byte("{\"name\":\"vol\",\"config\":{},\"properties\":{\"a\":\"b\"}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.CreateVolume(CreateVolumeRequest{Name: "foo/vol", Opts: map[string]interface{}{"a": "b"}})
	assert.Empty(t, resp.Err)
}

func TestCreateVolumeNoOpts(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes")
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, string(body), "{\"name\":\"vol\",\"properties\":{}}\n")
		_, _ = w.Write([]byte("{\"name\":\"vol\",\"config\":{},\"properties\":{}}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.CreateVolume(CreateVolumeRequest{Name: "foo/vol"})
	assert.Empty(t, resp.Err)
}

func TestCreateVolumeBadName(t *testing.T) {
	f := New("localhost", 5001)

	resp := f.CreateVolume(CreateVolumeRequest{Name: "foo", Opts: map[string]interface{}{"a": "b"}})
	assert.Equal(t, resp.Err, "volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

func TestCreateVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such repository\"}"))
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes")
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.CreateVolume(CreateVolumeRequest{Name: "foo/vol", Opts: map[string]interface{}{"a": "b"}})
	assert.Equal(t, resp.Err, "no such repository")
}

func TestRemoveVolume(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.Method, "DELETE")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
		w.WriteHeader(http.StatusNoContent)
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.RemoveVolume(VolumeRequest{Name: "foo/vol"})
	assert.Empty(t, resp.Err)
}

func TestRemoveVolumeBadName(t *testing.T) {
	f := New("localhost", 5001)

	resp := f.RemoveVolume(VolumeRequest{Name: "foo"})
	assert.Equal(t, resp.Err, "volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

func TestRemoveVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such repository\"}"))
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol")
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.RemoveVolume(VolumeRequest{Name: "foo/vol"})
	assert.Equal(t, resp.Err, "no such repository")
}

func TestMountVolume(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.RequestURI == "/v1/repositories/foo/volumes/vol" {
			assert.Equal(t, r.Method, "GET")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{\"name\":\"vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/vol\"}}"))
		} else {
			assert.Equal(t, r.Method, "POST")
			assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol/activate")
			w.WriteHeader(http.StatusNoContent)
		}
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.MountVolume(MountVolumeRequest{Name: "foo/vol"})
	assert.Empty(t, resp.Err)
}

func TestMountVolumeBadName(t *testing.T) {
	f := New("localhost", 5001)

	resp := f.MountVolume(MountVolumeRequest{Name: "foo"})
	assert.Equal(t, resp.Err, "volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

func TestMountVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.RequestURI == "/v1/repositories/foo/volumes/vol" {
			assert.Equal(t, r.Method, "GET")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{\"name\":\"vol\",\"properties\":{},\"config\":{\"mountpoint\":\"/vol\"}}"))
		} else {
			assert.Equal(t, r.Method, "POST")
			assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol/activate")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("{\"message\":\"no such repository\"}"))
		}
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.MountVolume(MountVolumeRequest{Name: "foo/vol"})
	assert.Equal(t, resp.Err, "no such repository")
}

func TestUnmountVolume(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol/deactivate")
		w.WriteHeader(http.StatusNoContent)
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.UnmountVolume(MountVolumeRequest{Name: "foo/vol"})
	assert.Empty(t, resp.Err)
}

func TestUnmountVolumeBadName(t *testing.T) {
	f := New("localhost", 5001)

	resp := f.UnmountVolume(MountVolumeRequest{Name: "foo"})
	assert.Equal(t, resp.Err, "volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

func TestUnmountVolumeError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.RequestURI, "/v1/repositories/foo/volumes/vol/deactivate")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("{\"message\":\"no such repository\"}"))
	})
	f, teardown := testForwarder(h)
	defer teardown()

	resp := f.UnmountVolume(MountVolumeRequest{Name: "foo/vol"})
	assert.Equal(t, resp.Err, "no such repository")
}
