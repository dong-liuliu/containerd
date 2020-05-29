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

package spdkvhost

import (
	//"encoding/json"
	"fmt"
	"os"

	//"os/exec"
	"path"
	//"strconv"
	//"strings"
	"context"

	"github.com/containerd/containerd/snapshots/devmapper/dmsetup"
	spdk "github.com/dong-liuliu/spdkctrl"
	//"github.com/pkg/errors"
	//"golang.org/x/sys/unix"
)

const (
	SectorSize = 512
	//SpdkRpcPath = "/home/xliu2/spdk-repos/spdk-intr/scripts/rpc.py"
	//spdkLvolBdevSize = 8192 //MB

	spdkAppPath       = "/root/go/src/github.com/spdk/spdk/app/vhost/vhost"
	spdkAppSocketPath = "/var/tmp/spdk.sock"
	spdkVhostSockPath = "/var/run/kata-containers/vhost-user/block/sockets/"
	vhostBlkPrefix    = "vhostblk-"
	//snapshotPrefix    = "snap-"
	//clonePrefix       = "clone-"
)

type SpdkProvider struct {
	client *spdk.Client
}

var spdkApp *spdk.App

func NewSpdkProvider() (*SpdkProvider, error) {
	var err error

	if false {
		optsFunc := []spdk.AppOption{}

		optsFunc = append(optsFunc, spdk.WithSpdkApp(spdkAppPath))
		optsFunc = append(optsFunc, spdk.WithAppSocket(spdkAppSocketPath))
		optsFunc = append(optsFunc, spdk.WithVhostSockPath(spdkVhostSockPath))
		//optsFunc = append(optsFunc, spdk.WithLogOutput(os.Stdout))

		spdkApp, err = spdk.AppRun(optsFunc...)
		err = nil
		if err != nil {
			return nil, err
		}
	}

	spdkClient, err := spdk.NewClient(spdkAppSocketPath, os.Stdout)
	if err != nil {
		return nil, err
	}

	return &SpdkProvider{client: spdkClient}, nil
}

func (v *SpdkProvider) Close() {
	v.client.Close()
	//spdk.AppTerm(spdkApp)
}

func (v *SpdkProvider) ProviderName() string {
	return "spdkvhost"
}

func (v *SpdkProvider) SectorSize() uint32 {
	return SectorSize
}

// CreatePool creates a device with the given name, data and metadata file and block size
// Parameter example: poolName = "containerd image pool"; dataFile = "/dev/nvme0n1"
func (v *SpdkProvider) CreatePool(poolName, dataFile, metaFile string, blockSizeSectors uint32) error {
	var err error

	_, err = spdk.BdevAioCreate(context.Background(), v.client,
		spdk.BdevAioCreateArgs{
			Name:      poolName,
			Filename:  dataFile,
			BlockSize: SectorSize})

	if err != nil {
		return err
	}

	// TODO: clear the bdevs automatically examined
	_, err = spdk.BdevLvolCreateLvstore(context.Background(), v.client,
		spdk.BdevLvolCreateLvstoreArgs{
			BdevName: poolName,
			LvsName:  poolName})

	return err
}

// ReloadPool reloads existing thin-pool (see "bdev_aio_create", and auto examine)
func (v *SpdkProvider) ReloadPool(poolName, dataFile, metaFile string, blockSizeSectors uint32) error {
	var err error

	_, err = spdk.BdevAioCreate(context.Background(), v.client,
		spdk.BdevAioCreateArgs{
			Name:      poolName,
			Filename:  dataFile,
			BlockSize: 4096})

	// TODO: clear the bdevs automatically examined
	return err
}

// CreateDevice sends "bdev_lvol_create -t -l LVS_NAME lvol_name size"
func (v *SpdkProvider) CreateDevice(poolName string, deviceID uint32, size uint64) error {
	var err error

	deviceIDStr := fmt.Sprintf("%d", deviceID)
	_, err = spdk.BdevLvolCreate(context.Background(), v.client,
		spdk.BdevLvolCreateArgs{
			LvolName:      deviceIDStr,
			Size:          int64(size / 1024 / 1024),
			ThinProvision: true,
			LvsName:       poolName})

	return err
}

// ActivateDevice activates the given thin-device using the 'thin' target
func (v *SpdkProvider) ActivateDevice(poolName string, deviceName string, deviceID uint32, size uint64, external string) error {
	var err error
	deviceIDStr := fmt.Sprintf("%d", deviceID)

	_, err = spdk.VhostCreateBlkController(context.Background(), v.client,
		spdk.VhostCreateBlkControllerArgs{
			DevName: poolName + "/" + deviceIDStr,
			Ctrlr:   vhostBlkPrefix + deviceName})

	return err
}

// SuspendDevice suspends the given device both read/write IO are pending there
func (v *SpdkProvider) SuspendDevice(deviceName string) error {
	return nil
}

// ResumeDevice resumes the given device
func (v *SpdkProvider) ResumeDevice(deviceName string) error {
	return nil
}

// CreateSnapshot sends "bdev_lvol_snapshot lvol_name snapshot name" message to the given thin-pool.
// ?Caller needs to suspend and resume device if it is active.
func (v *SpdkProvider) CreateSnapshot(poolName string, deviceID uint32, baseDeviceID uint32) error {
	var err error
	deviceIDStr := fmt.Sprintf("%d", deviceID)

	_, err = spdk.BdevLvolSnapshot(context.Background(), v.client,
		spdk.BdevLvolSnapshotArgs{
			LvolName:     poolName + "/" + fmt.Sprintf("%d", baseDeviceID),
			SnapshotName: deviceIDStr})

	return err
}

// delete a device
func (v *SpdkProvider) DeleteDevice(poolName string, deviceID uint32) error {
	var err error
	deviceIDStr := fmt.Sprintf("%d", deviceID)

	_, err = spdk.BdevLvolDelete(context.Background(), v.client,
		spdk.BdevLvolDeleteArgs{
			Name: poolName + "/" + deviceIDStr})

	return err
}

func (v *SpdkProvider) DeactivateDevice(deviceName string, opts ...dmsetup.DeactDeviceOpt) error {
	var err error

	_, err = spdk.VhostDeleteController(context.Background(), v.client,
		spdk.VhostDeleteControllerArgs{Ctrlr: vhostBlkPrefix + deviceName})

	return err
}

// Remove a pool
func (v *SpdkProvider) RemovePool(poolName string, opts ...dmsetup.DeactDeviceOpt) error {
	var err error

	_, err = spdk.BdevLvolDeleteLvstore(context.Background(), v.client,
		spdk.BdevLvolDeleteLvstoreArgs{
			LvsName: poolName})

	return err
}

/*
./scripts/rpc.py spdk_get_version
{
  "version": "SPDK v20.04-pre git sha1 2949499af",
  "fields": {
    "major": 20,
    "minor": 4,
    "patch": 0,
    "suffix": "-pre",
    "commit": "2949499af"
  }
}
*/
// Version returns "spdk_get_version" output
func (v *SpdkProvider) Version() (string, error) {
	return "20.X", nil
}

// For lvol bdev, return its vhost-blk socket patch
func (v *SpdkProvider) GetFullDevicePath(deviceName string) string {
	return path.Join(spdkVhostSockPath, vhostBlkPrefix+deviceName)
}

func (v *SpdkProvider) GetFullPoolPath(poolDir, pooName string) string {
	// TODO: specify pool path
	return "/dev/nvme0n1"
}

// Create nbd dev for host to use for the vhost device
func (v *SpdkProvider) DevHosting(devPath string) (string, error) {
	return "", nil
}

// Stop the nbd dev for host to use for the vhost device
func (v *SpdkProvider) UnDevHosting(devHostPath string) error {
	return nil
}

// TODO: fulfill or refactor Info function
// Info outputs device information (see "bdev_get_bdevs -b bdev_name").
// If device name is empty, all device infos will be returned.
func (v *SpdkProvider) Info(deviceName string) ([]*dmsetup.DeviceInfo, error) {
	//var devInfo []*dmsetup.DeviceInfo

	devInfo := make([]*dmsetup.DeviceInfo, 1)

	info := &dmsetup.DeviceInfo{}
	info.Name = deviceName

	_, err := spdk.VhostGetControllers(context.Background(), v.client,
		spdk.VhostGetControllersArgs{
			Name: vhostBlkPrefix + deviceName})
	if err != nil {
		info.TableLive = false
	} else {
		info.TableLive = true
	}

	devInfo[0] = info
	return devInfo, nil
}

func (v *SpdkProvider) InfoPool(poolName string) ([]*dmsetup.DeviceInfo, error) {
	//var devInfo []*dmsetup.DeviceInfo

	devInfo := make([]*dmsetup.DeviceInfo, 1)

	info := &dmsetup.DeviceInfo{}

	info.BlockDeviceName = "nvme0n1"
	info.Name = poolName
	//info.Major = 240,
	//info.Minor =

	devInfo[0] = info
	return devInfo, nil
}

func (v *SpdkProvider) GetUsage(deviceName string) (int64, error) {
	return 10240, nil
}

// Table returns the current table for the device
func (v *SpdkProvider) Table(deviceName string) (string, error) {
	return "", fmt.Errorf("Not implemented")
}

/*
func XXX() {
	poolName := v.PoolName
	if deviceName == PoolName {
		if true {
			devPath = "/dev/nvme1n1"
		} else {
			var err error

			respLvs, err := spdk.BdevLvolGetLvstores(context.Background(), v.client,
				spdk.BdevLvolGetLvstoresArgs{LvsName: poolName})
			if err {
				return ""
			}

			bdevName := respLvs[0].BaseBdev

			respBdev, err := spdk.BdevGetBdevs(context.Background(), v.client,
				spdk.BdevGetBdevsArgs{Name: bdevName})
			if err {
				return ""
			}
			//respBdev[0].DriverSpecific

			return ""
		}

		return devPath
	}

	return path.Join(spdkVhostSockPath, vhostBlkPrefix+deviceName)
}*/
