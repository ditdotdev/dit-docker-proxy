// Copyright Dit 2026
// SPDX-License-Identifier: BUSL-1.1

package forwarder

import (
	"context"
	"errors"
	"fmt"
	dit "github.com/ditdotdev/dit-client-go"
	"net/http"
	"regexp"
	"sync"
)

/*
 * The forwarder class is responsible for taking docker requests as input, and making the appropriate calls to an
 * instance of dit-server. The inputs to these functions are all structures defined in this package. The
 * responsibility of listening on the appropriate docker socket, marshalling to and from JSON, etc rests with
 * other portions of the package.
 */

type Forwarder interface {
	CreateVolume(request CreateVolumeRequest) VolumeResponse
	GetPath(request VolumeRequest) GetPathResponse
	GetVolume(request VolumeRequest) GetVolumeResponse
	ListVolumes() ListVolumeResponse
	MountVolume(request MountVolumeRequest) GetPathResponse
	PluginActivate() PluginDescription
	RemoveVolume(request VolumeRequest) VolumeResponse
	VolumeCapabilities() VolumeCapabilities
	UnmountVolume(request MountVolumeRequest) VolumeResponse
}

type forwarder struct {
	client *dit.APIClient
	ctx    context.Context
	mounts *mountTable
}

/*
 * Docker calls /VolumeDriver.Mount once per consumer of a volume and /VolumeDriver.Unmount once when that consumer
 * is done, and it expects the driver to keep the volume mounted until the last consumer has gone (this is what
 * moby's own "local" driver does, and it counts plainly rather than by ID because `docker cp` and `docker export`
 * against a running container reuse the container's own mount ID). Without this table a `docker cp` against a
 * running repository re-ran `mount` on an already mounted dataset, which fails, and its Unmount then pulled the
 * dataset out from under the running container (ditdotdev/dit#260).
 *
 * The table is keyed by the Docker volume name. The mountpoint is remembered on first activation so that further
 * references do not need a round trip to the server.
 */
type mountTable struct {
	mu      sync.Mutex
	entries map[string]*mountEntry
}

type mountEntry struct {
	references int
	mountpoint string
}

func newMountTable() *mountTable {
	return &mountTable{entries: map[string]*mountEntry{}}
}

/*
 * Converts an error object into an "Err" string to return to consumers. If this is a dit-server API error, then
 * we return the message field. Otherwise, we return the default error string.
 *
 * Note: as of dit-client-go v1.11.0 the generated code returns *GenericOpenAPIError
 * (pointer), not the value type. The pointer assertion is load-bearing — a value assertion
 * silently fails and every server-side error degrades from "no such repository" to bare HTTP
 * status text.
 */
func getErrorString(err error) string {
	if openApiErr, ok := err.(*dit.GenericOpenAPIError); ok {
		if apiErr, ok := openApiErr.Model().(dit.ApiError); ok {
			return apiErr.Message
		}
	}
	return err.Error()
}

// Underscore is the canonical separator (works on every platform Docker
// supports); slash is the older form we still accept for back-compat.
// Both regexes use a greedy second group so volume names with additional
// underscores (e.g. "my_repo_my_vol" → repo="my", vol="repo_my_vol")
// don't fall through to the error path. Repository names containing the
// separator are not supported — we always split on the first occurrence.
var (
	volumeNameUnderscoreRE = regexp.MustCompile(`^([^_]+)_(.+)$`)
	volumeNameSlashRE      = regexp.MustCompile(`^([^/]+)/(.+)$`)
)

/*
 * Converts a docker volume name (repo_vol) to a (repo, volume) tuple for use with the Dit API.
 * Uses underscore format for universal compatibility, with backward compatibility for existing slash format.
 */
func parseVolumeName(volumeName string) (string, string, error) {
	// Try underscore format first (new standard)
	if match := volumeNameUnderscoreRE.FindStringSubmatch(volumeName); len(match) == 3 {
		return match[1], match[2], nil
	}
	// Fall back to slash format for backward compatibility
	if match := volumeNameSlashRE.FindStringSubmatch(volumeName); len(match) == 3 {
		return match[1], match[2], nil
	}
	return "", "", errors.New("volume name must be of the form <repository>_<volume> or <repository>/<volume>")
}

/*
 * A number of methods return a common VolumeResponse, which contains only an "Err" field. This method will handle
 * an optional error and convert it to that common type.
 */
func standardResponse(err error) VolumeResponse {
	if err != nil {
		return VolumeResponse{Err: getErrorString(err)}
	}
	return VolumeResponse{}
}

/*
 * Converts from a Dit volume to a Docker volume. The main difference is that the repository name is part of the
 * volume name. The mountpoint is also pulled out of the properties to a first class response.
 * Uses underscore format for universal compatibility across all platforms.
 */
func convertVolume(repo string, vol dit.Volume) Volume {
	// vol.Config is an optional map[string]interface{}; defensive comma-ok
	// because the v1.11 generator emits it as omitempty and the server may
	// elide it entirely on List responses. A bare type assertion would
	// panic the request goroutine and Docker would see a TCP RST instead
	// of a JSON error.
	mp, _ := vol.Config["mountpoint"].(string)
	return Volume{
		Name:       fmt.Sprintf("%s_%s", repo, vol.Name),
		Mountpoint: mp,
		Status:     map[string]string{},
	}
}

/*
 * /VolumeDriver.Capabilities
 *
 * This always returns a static definition with a "local" scope.
 */
func (p forwarder) VolumeCapabilities() VolumeCapabilities {
	return VolumeCapabilities{Capabilities: Capability{Scope: "local"}}
}

/*
 * /VolumeDriver.List
 *
 * Returns a list of all volumes on the system. This requires iterating over all repositories followed by the volumes
 * for each.
 */
func (p forwarder) ListVolumes() ListVolumeResponse {
	repositories, _, err := p.client.RepositoriesApi.ListRepositories(p.ctx).Execute()
	if err != nil {
		return ListVolumeResponse{Err: getErrorString(err)}
	}

	ret := ListVolumeResponse{
		Volumes: []Volume{},
	}

	for _, repo := range repositories {
		volumes, _, err := p.client.VolumesApi.ListVolumes(p.ctx, repo.Name).Execute()
		if err != nil {
			return ListVolumeResponse{Err: getErrorString(err)}
		}
		for _, vol := range volumes {
			ret.Volumes = append(ret.Volumes, convertVolume(repo.Name, vol))
		}
	}

	return ret
}

/*
 * /Plugin.Activate
 *
 * This always returns a static definition implementing "VolumeDriver"
 */
func (p forwarder) PluginActivate() PluginDescription {
	return PluginDescription{
		Implements: []string{"VolumeDriver"},
	}
}

/*
 * /VolumeDriver.Get
 *
 * Get a single volume.
 */
func (p forwarder) GetVolume(request VolumeRequest) GetVolumeResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err != nil {
		return GetVolumeResponse{Err: getErrorString(err)}
	}

	volume, _, err := p.client.VolumesApi.GetVolume(p.ctx, repoName, volumeName).Execute()
	if err != nil {
		return GetVolumeResponse{Err: getErrorString(err)}
	}

	return GetVolumeResponse{Volume: convertVolume(repoName, *volume)}
}

/*
 * /VolumeDriver.Path
 *
 * Get the mountpoint for a volume. Equivalent to getting the mountpoint member of the volume.
 */
func (p forwarder) GetPath(request VolumeRequest) GetPathResponse {
	vol := p.GetVolume(request)
	if vol.Err != "" {
		return GetPathResponse{Err: vol.Err}
	}
	return GetPathResponse{Mountpoint: vol.Volume.Mountpoint}
}

/*
 * /VolumeDriver.Create
 *
 * Create a new repository. The "Opts" map is converted to be the volume properties.
 */
func (p forwarder) CreateVolume(request CreateVolumeRequest) VolumeResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err == nil {
		properties := map[string]interface{}{}
		if request.Opts != nil {
			properties = request.Opts
		}
		vol := dit.Volume{
			Name:       volumeName,
			Properties: properties,
		}
		_, _, err = p.client.VolumesApi.CreateVolume(p.ctx, repoName).Volume(vol).Execute()
	}
	return standardResponse(err)
}

/*
 * /VolumeDriver.Remove
 *
 * Delete a volume. This simply parses the name to the native Dit form, and marshals any errors in the process.
 */
func (p forwarder) RemoveVolume(request VolumeRequest) VolumeResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err != nil {
		return standardResponse(err)
	}

	_, err = p.client.VolumesApi.DeleteVolume(p.ctx, repoName, volumeName).Execute()
	return standardResponse(err)
}

/*
 * /VolumeDriver.Mount
 *
 * Mount a volume. The first reference activates the Dit volume; further references (a `docker cp` against a
 * running container, for example) just bump the count and return the mountpoint recorded at activation. The table
 * lock is held across the server calls so that concurrent first references cannot both activate.
 */
func (p forwarder) MountVolume(request MountVolumeRequest) GetPathResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err != nil {
		return GetPathResponse{Err: getErrorString(err)}
	}

	p.mounts.mu.Lock()
	defer p.mounts.mu.Unlock()

	if entry, ok := p.mounts.entries[request.Name]; ok && entry.references > 0 {
		entry.references++
		return GetPathResponse{Mountpoint: entry.mountpoint}
	}

	vol, _, err := p.client.VolumesApi.GetVolume(p.ctx, repoName, volumeName).Execute()
	if err == nil {
		_, err = p.client.VolumesApi.ActivateVolume(p.ctx, repoName, volumeName).Execute()
	}
	if err != nil {
		return GetPathResponse{Err: getErrorString(err)}
	}

	// vol is non-nil when err == nil (v1.11 client invariant).
	// Defensive comma-ok on the map value matches convertVolume.
	mp, _ := vol.Config["mountpoint"].(string)
	p.mounts.entries[request.Name] = &mountEntry{references: 1, mountpoint: mp}
	return GetPathResponse{Mountpoint: mp}
}

/*
 * /VolumeDriver.Unmount
 *
 * Unmount a volume. Drops one reference and deactivates the Dit volume only when no references remain. An Unmount
 * the table knows nothing about (for example after the plugin restarted underneath a running container) still
 * deactivates, as it always did, so that a volume cannot get stuck mounted. The reference is dropped even if the
 * deactivation fails, matching moby's local driver: Docker reports the error, and a retried Unmount runs the
 * deactivation again instead of returning a success that did not happen.
 */
func (p forwarder) UnmountVolume(request MountVolumeRequest) VolumeResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err != nil {
		return standardResponse(err)
	}

	p.mounts.mu.Lock()
	defer p.mounts.mu.Unlock()

	if entry, ok := p.mounts.entries[request.Name]; ok && entry.references > 0 {
		entry.references--
		if entry.references > 0 {
			return VolumeResponse{}
		}
		delete(p.mounts.entries, request.Name)
	}

	_, err = p.client.VolumesApi.DeactivateVolume(p.ctx, repoName, volumeName).Execute()
	return standardResponse(err)
}

/*
 * Public forwarder constructor. Takes a host ("localhost") and port (5001) to pass to the client.
 *
 * Note: in dit-client-go v1.11.0 the Configuration.Host field is no
 * longer used to drive request URLs (that's now the Servers slice).
 * Setting Host alone silently sent every request to the default
 * http://localhost:5001 server. We override Servers[0].URL instead.
 */
func New(host string, port int) Forwarder {
	config := dit.NewConfiguration()
	config.Servers = dit.ServerConfigurations{
		{URL: fmt.Sprintf("http://%s:%d", host, port)},
	}
	client := dit.NewAPIClient(config)
	return forwarder{
		client: client,
		ctx:    context.Background(),
		mounts: newMountTable(),
	}
}

/*
 * For use in testing, this allows the test to pass a (mock) HTTP client to the Dit client in order to facilitate
 * testing.
 */
func NewClient(httpClient *http.Client) Forwarder {
	config := dit.NewConfiguration()
	config.HTTPClient = httpClient
	client := dit.NewAPIClient(config)
	return forwarder{
		client: client,
		ctx:    context.Background(),
		mounts: newMountTable(),
	}
}
