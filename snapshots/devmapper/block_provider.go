// +build linux

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package devmapper

import (
	"fmt"

	dm "github.com/containerd/containerd/snapshots/devmapper/dmsetup"
	spdk "github.com/containerd/containerd/snapshots/devmapper/spdkvhost"
)

type blockProvider interface {
	ProviderName() string
	Close()

	// CreatePool creates a device with the given name, data and metadata file and block size (see "dmsetup create")
	CreatePool(poolName, dataFile, metaFile string, blockSizeSectors uint32) error
	// ReloadPool reloads existing thin-pool (see "dmsetup reload")
	ReloadPool(PoolName, dataFile, metaFile string, blockSizeSectors uint32) error
	// Remove a pool
	// RemovePool removes the pool device
	RemovePool(PoolName string, opts ...dm.DeactDeviceOpt) error

	// CreateDevice sends "create_thin <deviceID>" message to the given thin-pool
	CreateDevice(poolName string, deviceID uint32, size uint64) error
	// CreateSnapshot sends "create_snap" message to the given thin-pool.
	// Caller needs to suspend and resume device if it is active.
	CreateSnapshot(poolName string, deviceID uint32, baseDeviceID uint32) error
	// DeleteDevice sends "delete <deviceID>" message to the given thin-pool
	DeleteDevice(poolName string, deviceID uint32) error

	// ActivateDevice activates the given thin-device using the 'thin' target
	ActivateDevice(poolName string, deviceName string, deviceID uint32, size uint64, external string) error
	// DeactivateDevice removes a device (see "dmsetup remove")
	DeactivateDevice(deviceName string, opts ...dm.DeactDeviceOpt) error

	// SuspendDevice suspends the given device (see "dmsetup suspend")
	SuspendDevice(deviceName string) error
	// ResumeDevice resumes the given device (see "dmsetup resume")
	ResumeDevice(deviceName string) error

	// Version returns "dmsetup version" output
	Version() (string, error)
	// GetFullDevicePath returns full path for the given device name (like "/dev/mapper/name", or "<vhost-socket-dir>/vhost-socket-name")
	GetFullDevicePath(deviceName string) string
	// GetFullDevicePath returns full path for the given pool name (like "/dev/mapper/pool-name", or "/<pool-device-path>/<pool-device-name>")
	GetFullPoolPath(poolDir, poolName string) string

	// Info outputs device information (see "dmsetup info").
	// If device name is empty, all device infos will be returned.
	Info(deviceName string) ([]*dm.DeviceInfo, error)
	InfoPool(poolName string) ([]*dm.DeviceInfo, error)

	DevHosting(devPath string) (string, error)
	UnDevHosting(devHostPath string) error

	//Seems it is only used inside dmsetup
	// BlockDeviceSize returns size of block device in bytes
	//BlockDeviceSize(devicePath string) (uint64, error)
	SectorSize() uint32

	GetUsage(deviceName string) (int64, error)

	//Seems it is not used outside
	// Table returns the current table for the device
	Table(deviceName string) (string, error)
}

func GetBlockProvider(providerName string) (blockProvider, error) {
	if providerName == "dmsetup" || providerName == "" {
		return &dm.DmProvider{}, nil
	} else if providerName == "spdkvhost" || providerName == "spdk_vhost" {
		return spdk.NewSpdkProvider()
	}

	return nil, fmt.Errorf("Invalid block provider name")
}
