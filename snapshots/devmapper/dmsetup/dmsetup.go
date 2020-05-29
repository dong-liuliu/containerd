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

package dmsetup

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	// DevMapperDir represents devmapper devices location
	DevMapperDir = "/dev/mapper/"
	// SectorSize represents the number of bytes in one sector on devmapper devices
	SectorSize = 512
)

type DmProvider struct{}

// DeviceInfo represents device info returned by "dmsetup info".
// dmsetup(8) provides more information on each of these fields.
type DeviceInfo struct {
	Name            string
	BlockDeviceName string
	TableLive       bool
	TableInactive   bool
	Suspended       bool
	ReadOnly        bool
	Major           uint32
	Minor           uint32
	OpenCount       uint32 // Open reference count
	TargetCount     uint32 // Number of targets in the live table
	EventNumber     uint32 // Last event sequence number (used by wait)
}

var errTable map[string]unix.Errno

func init() {
	// Precompute map of <text>=<errno> for optimal lookup
	errTable = make(map[string]unix.Errno)
	for errno := unix.EPERM; errno <= unix.EHWPOISON; errno++ {
		errTable[errno.Error()] = errno
	}
}

func (dm *DmProvider) Close() {
}
func (dm *DmProvider) ProviderName() string {
	return "dmsetup"
}
func (dm *DmProvider) DevHosting(devPath string) (string, error) {
	return devPath, nil
}
func (dm *DmProvider) UnDevHosting(devHostPath string) error {
	return nil
}
func (dm *DmProvider) GetFullPoolPath(poolDir, poolName string) string {
	return dm.GetFullDevicePath(poolName)
}

func (dm *DmProvider) RemovePool(PoolName string, opts ...DeactDeviceOpt) error {
	return dm.DeactivateDevice(PoolName, opts...)
}

func (dm *DmProvider) SectorSize() uint32 {
	return SectorSize
}

// CreatePool creates a device with the given name, data and metadata file and block size (see "dmsetup create")
func (dm *DmProvider) CreatePool(poolName, dataFile, metaFile string, blockSizeSectors uint32) error {
	thinPool, err := makeThinPoolMapping(dataFile, metaFile, blockSizeSectors)
	if err != nil {
		return err
	}

	_, err = dmsetup("create", poolName, "--table", thinPool)
	return err
}

// ReloadPool reloads existing thin-pool (see "dmsetup reload")
func (dm *DmProvider) ReloadPool(deviceName, dataFile, metaFile string, blockSizeSectors uint32) error {
	thinPool, err := makeThinPoolMapping(dataFile, metaFile, blockSizeSectors)
	if err != nil {
		return err
	}

	_, err = dmsetup("reload", deviceName, "--table", thinPool)
	return err
}

const (
	lowWaterMark = 32768                // Picked arbitrary, might need tuning
	skipZeroing  = "skip_block_zeroing" // Skipping zeroing to reduce latency for device creation
)

// makeThinPoolMapping makes thin-pool table entry
func makeThinPoolMapping(dataFile, metaFile string, blockSizeSectors uint32) (string, error) {
	dataDeviceSizeBytes, err := BlockDeviceSize(dataFile)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get block device size: %s", dataFile)
	}

	// Thin-pool mapping target has the following format:
	// start - starting block in virtual device
	// length - length of this segment
	// metadata_dev - the metadata device
	// data_dev - the data device
	// data_block_size - the data block size in sectors
	// low_water_mark - the low water mark, expressed in blocks of size data_block_size
	// feature_args - the number of feature arguments
	// args
	lengthSectors := dataDeviceSizeBytes / SectorSize
	target := fmt.Sprintf("0 %d thin-pool %s %s %d %d 1 %s",
		lengthSectors,
		metaFile,
		dataFile,
		blockSizeSectors,
		lowWaterMark,
		skipZeroing)

	return target, nil
}

// CreateDevice sends "create_thin <deviceID>" message to the given thin-pool
func (dm *DmProvider) CreateDevice(poolName string, deviceID uint32, size uint64) error {
	_, err := dmsetup("message", poolName, "0", fmt.Sprintf("create_thin %d", deviceID))
	return err
}

// ActivateDevice activates the given thin-device using the 'thin' target
func (dm *DmProvider) ActivateDevice(poolName string, deviceName string, deviceID uint32, size uint64, external string) error {
	mapping := makeThinMapping(poolName, deviceID, size, external)
	_, err := dmsetup("create", deviceName, "--table", mapping)
	return err
}

// makeThinMapping makes thin target table entry
func makeThinMapping(poolName string, deviceID uint32, sizeBytes uint64, externalOriginDevice string) string {
	lengthSectors := sizeBytes / SectorSize
	var dm *DmProvider

	// Thin target has the following format:
	// start - starting block in virtual device
	// length - length of this segment
	// pool_dev - the thin-pool device, can be /dev/mapper/pool_name or 253:0
	// dev_id - the internal device id of the device to be activated
	// external_origin_dev - an optional block device outside the pool to be treated as a read-only snapshot origin.
	target := fmt.Sprintf("0 %d thin %s %d %s", lengthSectors, dm.GetFullDevicePath(poolName), deviceID, externalOriginDevice)
	return strings.TrimSpace(target)
}

// SuspendDevice suspends the given device (see "dmsetup suspend")
func (dm *DmProvider) SuspendDevice(deviceName string) error {
	_, err := dmsetup("suspend", deviceName)
	return err
}

// ResumeDevice resumes the given device (see "dmsetup resume")
func (dm *DmProvider) ResumeDevice(deviceName string) error {
	_, err := dmsetup("resume", deviceName)
	return err
}

// Table returns the current table for the device
func (dm *DmProvider) Table(deviceName string) (string, error) {
	return dmsetup("table", deviceName)
}

// CreateSnapshot sends "create_snap" message to the given thin-pool.
// Caller needs to suspend and resume device if it is active.
func (dm *DmProvider) CreateSnapshot(poolName string, deviceID uint32, baseDeviceID uint32) error {
	_, err := dmsetup("message", poolName, "0", fmt.Sprintf("create_snap %d %d", deviceID, baseDeviceID))
	return err
}

// DeleteDevice sends "delete <deviceID>" message to the given thin-pool
func (dm *DmProvider) DeleteDevice(poolName string, deviceID uint32) error {
	_, err := dmsetup("message", poolName, "0", fmt.Sprintf("delete %d", deviceID))
	return err
}

// DeactDeviceOpt represents command line arguments for "dmsetup remove" command
type DeactDeviceOpt string

const (
	// RemoveWithForce flag replaces the table with one that fails all I/O if
	// open device can't be removed
	RemoveWithForce DeactDeviceOpt = "--force"
	// RemoveWithRetries option will cause the operation to be retried
	// for a few seconds before failing
	RemoveWithRetries DeactDeviceOpt = "--retry"
	// RemoveDeferred flag will enable deferred removal of open devices,
	// the device will be removed when the last user closes it
	RemoveDeferred DeactDeviceOpt = "--deferred"
)

// DeactivateDevice removes a device (see "dmsetup remove")
func (dm *DmProvider) DeactivateDevice(deviceName string, opts ...DeactDeviceOpt) error {
	args := []string{
		"remove",
	}

	for _, opt := range opts {
		args = append(args, string(opt))
	}

	args = append(args, dm.GetFullDevicePath(deviceName))

	_, err := dmsetup(args...)
	if err == unix.ENXIO {
		// Ignore "No such device or address" error because we dmsetup
		// remove with "deferred" option, there is chance for the device
		// having been removed.
		return nil
	}

	return err
}

//#dmsetup info --columns --noheadings -o name,blkdevname,attr,major,minor,open,segments,events --separator " "
//fedora-swap dm-1 L--w 253 1 2 1 0
// Info outputs device information (see "dmsetup info").
// If device name is empty, all device infos will be returned.
// Info outputs device information (see "dmsetup info").
// If device name is empty, all device infos will be returned.
func (dm *DmProvider) Info(deviceName string) ([]*DeviceInfo, error) {
	output, err := dmsetup(
		"info",
		"--columns",
		"--noheadings",
		"-o",
		"name,blkdevname,attr,major,minor,open,segments,events",
		"--separator",
		" ",
		deviceName)

	if err != nil {
		return nil, err
	}

	var (
		lines   = strings.Split(output, "\n")
		devices = make([]*DeviceInfo, len(lines))
	)

	for i, line := range lines {
		var (
			attr = ""
			info = &DeviceInfo{}
		)

		_, err := fmt.Sscan(line,
			&info.Name,
			&info.BlockDeviceName,
			&attr,
			&info.Major,
			&info.Minor,
			&info.OpenCount,
			&info.TargetCount,
			&info.EventNumber)

		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse line %q", line)
		}

		// Parse attributes (see "man 8 dmsetup" for details)
		info.Suspended = strings.Contains(attr, "s")
		info.ReadOnly = strings.Contains(attr, "r")
		info.TableLive = strings.Contains(attr, "L")
		info.TableInactive = strings.Contains(attr, "I")

		devices[i] = info
	}

	return devices, nil
}
func (dm *DmProvider) InfoPool(deviceName string) ([]*DeviceInfo, error) {
	return dm.Info(deviceName)
}

// Version returns "dmsetup version" output
func (dm *DmProvider) Version() (string, error) {
	return dmsetup("version")
}

// DeviceStatus represents devmapper device status information
type DeviceStatus struct {
	Offset int64
	Length int64
	Target string
	Params []string
}

// Status provides status information for devmapper device
func status(deviceName string) (*DeviceStatus, error) {
	var (
		err    error
		status DeviceStatus
	)

	output, err := dmsetup("status", deviceName)
	if err != nil {
		return nil, err
	}

	// Status output format:
	//  Offset (int64)
	//  Length (int64)
	//  Target type (string)
	//  Params (Array of strings)
	const MinParseCount = 4
	parts := strings.Split(output, " ")
	if len(parts) < MinParseCount {
		return nil, errors.Errorf("failed to parse output: %q", output)
	}

	status.Offset, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse offset: %q", parts[0])
	}

	status.Length, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse length: %q", parts[1])
	}

	status.Target = parts[2]
	status.Params = parts[3:]

	return &status, nil
}

// GetUsage reports total size in bytes consumed by a thin-device.
// It relies on the number of used blocks reported by 'dmsetup status'.
// The output looks like:
//  device2: 0 204800 thin 17280 204799
// Where 17280 is the number of used sectors
func (dm *DmProvider) GetUsage(deviceName string) (int64, error) {

	status, err := status(deviceName)
	if err != nil {
		return 0, errors.Wrapf(err, "can't get status for device %q", deviceName)
	}

	if len(status.Params) == 0 {
		return 0, errors.Errorf("failed to get the number of used blocks, unexpected output from dmsetup status")
	}

	count, err := strconv.ParseInt(status.Params[0], 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse status params: %q", status.Params[0])
	}

	return count * SectorSize, nil
}

// GetFullDevicePath returns full path for the given device name (like "/dev/mapper/name")
func (dm *DmProvider) GetFullDevicePath(deviceName string) string {
	if strings.HasPrefix(deviceName, DevMapperDir) {
		return deviceName
	}

	return DevMapperDir + deviceName
}

// BlockDeviceSize returns size of block device in bytes
func BlockDeviceSize(devicePath string) (uint64, error) {
	data, err := exec.Command("blockdev", "--getsize64", "-q", devicePath).CombinedOutput()
	output := string(data)
	if err != nil {
		return 0, errors.Wrapf(err, output)
	}

	output = strings.TrimSuffix(output, "\n")
	return strconv.ParseUint(output, 10, 64)
}

func dmsetup(args ...string) (string, error) {
	data, err := exec.Command("dmsetup", args...).CombinedOutput()
	output := string(data)
	if err != nil {
		// Try find Linux error code otherwise return generic error with dmsetup output
		if errno, ok := tryGetUnixError(output); ok {
			return "", errno
		}

		return "", errors.Wrapf(err, "dmsetup %s\nerror: %s\n", strings.Join(args, " "), output)
	}

	output = strings.TrimSuffix(output, "\n")
	output = strings.TrimSpace(output)

	return output, nil
}

// tryGetUnixError tries to find Linux error code from dmsetup output
func tryGetUnixError(output string) (unix.Errno, bool) {
	// It's useful to have Linux error codes like EBUSY, EPERM, ..., instead of just text.
	// Unfortunately there is no better way than extracting/comparing error text.
	text := parseDmsetupError(output)
	if text == "" {
		return 0, false
	}

	err, ok := errTable[text]
	return err, ok
}

// dmsetup returns error messages in format:
// 	device-mapper: message ioctl on <name> failed: File exists\n
// 	Command failed\n
// parseDmsetupError extracts text between "failed: " and "\n"
func parseDmsetupError(output string) string {
	lines := strings.SplitN(output, "\n", 2)
	if len(lines) < 2 {
		return ""
	}

	line := lines[0]
	// Handle output like "Device /dev/mapper/snapshotter-suite-pool-snap-1 not found"
	if strings.HasSuffix(line, "not found") {
		return unix.ENXIO.Error()
	}

	const failedSubstr = "failed: "
	idx := strings.LastIndex(line, failedSubstr)
	if idx == -1 {
		return ""
	}

	str := line[idx:]

	// Strip "failed: " prefix
	str = strings.TrimPrefix(str, failedSubstr)

	str = strings.ToLower(str)
	return str
}
