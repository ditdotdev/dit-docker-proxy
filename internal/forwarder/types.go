/*
 * Copyright Datadatdat.
 */

package forwarder

type Capability struct {
	Scope string
}

type CreateVolumeRequest struct {
	Name string
	Opts map[string]interface{}
}

type GetPathResponse struct {
	Err        string
	Mountpoint string
}

type GetVolumeResponse struct {
	Err    string
	Volume Volume
}

type ListVolumeResponse struct {
	Err     string
	Volumes []Volume
}

type MountVolumeRequest struct {
	Name string
	ID   string
}

type PluginDescription struct {
	Implements []string
}

type Volume struct {
	Name       string
	Mountpoint string
	Status     map[string]string
}

type VolumeCapabilities struct {
	Capabilities Capability
}

type VolumeRequest struct {
	Name string
}

type VolumeResponse struct {
	Err string
}
