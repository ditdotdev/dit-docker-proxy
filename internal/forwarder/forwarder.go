/*
 * Copyright Datadatdat.
 */

package forwarder

import (
	"context"
	"errors"
	"fmt"
	datadatdat "github.com/datadatdat/datadatdat-client-go"
	"net/http"
	"regexp"
)

/*
 * The forwarder class is responsible for taking docker requests as input, and making the appropriate calls to an
 * instance of datadatdat-server. The inputs to these functions are all structures defined in this package. The
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
	client *datadatdat.APIClient
	ctx    context.Context
}

/*
 * Converts an error object into an "Err" string to return to consumers. If this is a datadatdat-server API error, then
 * we return the message field. Otherwise, we return the default error string.
 *
 * Note: as of datadatdat-client-go v1.11.0 the generated code returns *GenericOpenAPIError
 * (pointer), not the value type. The pointer assertion is load-bearing — a value assertion
 * silently fails and every server-side error degrades from "no such repository" to bare HTTP
 * status text.
 */
func getErrorString(err error) string {
	if openApiErr, ok := err.(*datadatdat.GenericOpenAPIError); ok {
		if apiErr, ok := openApiErr.Model().(datadatdat.ApiError); ok {
			return apiErr.Message
		}
	}
	return err.Error()
}

/*
 * Converts a docker volume name (repo_vol) to a (repo, volume) tuple for use with the Datadatdat API.
 * Uses underscore format for universal compatibility, with backward compatibility for existing slash format.
 */
func parseVolumeName(volumeName string) (string, string, error) {
	// Try underscore format first (new standard)
	re := regexp.MustCompile(`^([^_]+)_([^_]+)$`)
	match := re.FindStringSubmatch(volumeName)
	if len(match) == 3 {
		return match[1], match[2], nil
	}
	
	// Fall back to slash format for backward compatibility
	re = regexp.MustCompile(`^([^/]+)/([^/]+)$`)
	match = re.FindStringSubmatch(volumeName)
	if len(match) == 3 {
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
 * Converts from a Datadatdat volume to a Docker volume. The main difference is that the repository name is part of the
 * volume name. The mountpoint is also pulled out of the properties to a first class response.
 * Uses underscore format for universal compatibility across all platforms.
 */
func convertVolume(repo string, vol datadatdat.Volume) Volume {
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
		vol := datadatdat.Volume{
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
 * Delete a volume. This simply parses the name to the native Datadatdat form, and marshals any errors in the process.
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
 * Mount a volume. This is equivalent to activating a Datadatdat volume.
 */
func (p forwarder) MountVolume(request MountVolumeRequest) GetPathResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err == nil {
		var vol *datadatdat.Volume
		vol, _, err = p.client.VolumesApi.GetVolume(p.ctx, repoName, volumeName).Execute()
		if err == nil {
			_, err = p.client.VolumesApi.ActivateVolume(p.ctx, repoName, volumeName).Execute()
		}
		if err == nil {
			// vol is non-nil when err == nil (v1.11 client invariant).
			// Defensive comma-ok on the map value matches convertVolume.
			mp, _ := vol.Config["mountpoint"].(string)
			return GetPathResponse{Mountpoint: mp}
		}
	}

	return GetPathResponse{Err: getErrorString(err)}
}

/*
 * /VolumeDriver.Unmount
 *
 * Unmount a volume. This is equivalent to deactivating a Datadatdat volume.
 */
func (p forwarder) UnmountVolume(request MountVolumeRequest) VolumeResponse {
	repoName, volumeName, err := parseVolumeName(request.Name)
	if err == nil {
		_, err = p.client.VolumesApi.DeactivateVolume(p.ctx, repoName, volumeName).Execute()
	}
	return standardResponse(err)
}

/*
 * Public forwarder constructor. Takes a host ("localhost") and port (5001) to pass to the client.
 *
 * Note: in datadatdat-client-go v1.11.0 the Configuration.Host field is no
 * longer used to drive request URLs (that's now the Servers slice).
 * Setting Host alone silently sent every request to the default
 * http://localhost:5001 server. We override Servers[0].URL instead.
 */
func New(host string, port int) Forwarder {
	config := datadatdat.NewConfiguration()
	config.Servers = datadatdat.ServerConfigurations{
		{URL: fmt.Sprintf("http://%s:%d", host, port)},
	}
	client := datadatdat.NewAPIClient(config)
	return forwarder{
		client: client,
		ctx:    context.Background(),
	}
}

/*
 * For use in testing, this allows the test to pass a (mock) HTTP client to the Datadatdat client in order to facilitate
 * testing.
 */
func NewClient(httpClient *http.Client) Forwarder {
	config := datadatdat.NewConfiguration()
	config.HTTPClient = httpClient
	client := datadatdat.NewAPIClient(config)
	return forwarder{
		client: client,
		ctx:    context.Background(),
	}
}
