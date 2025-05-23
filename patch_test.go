package magiskboot_test

import (
	"bytes"
	"io"
	"magiskboot"
	"os"
	"testing"
)

func TestHexPatch(t *testing.T) {
	t.Log("Test HexPatch function")
	os.Remove("test.bin")
	if fd, err := os.Create("test.bin"); err != nil {
		t.Fatal(err)

	} else {
		fd.WriteString("12345678901234567890")
		fd.Close()
		magiskboot.HexPatch("test.bin", "31323334", "35363738")
		expect := []byte("56785678905678567890")
		if fd, err = os.Open("test.bin"); err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(fd)
		if !bytes.Equal(data, expect) {
			t.Fatalf("Except: %v\nBut: %v", expect, data)
		}
		defer fd.Close()
		defer os.Remove("test.bin")
	}
}

func TestRemovePatterns(t *testing.T) {
	t.Log("Test remove patterns function")
	tdata := []byte(`
# Copyright (c) 2018, The Linux Foundation. All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are
# met:
#     * Redistributions of source code must retain the above copyright
#       notice, this list of conditions and the following disclaimer.
#     * Redistributions in binary form must reproduce the above
#       copyright notice, this list of conditions and the following
#       disclaimer in the documentation and/or other materials provided
#       with the distribution.
#     * Neither the name of The Linux Foundation nor the names of its
#       contributors may be used to endorse or promote products derived
#       from this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED "AS IS" AND ANY EXPRESS OR IMPLIED
# WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
# MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NON-INFRINGEMENT
# ARE DISCLAIMED.  IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS
# BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR
# BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
# WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE
# OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN
# IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

# Android fstab file.
# The filesystem that contains the filesystem checker binary (typically /system) cannot
# specify MF_CHECK, and must come before any filesystems that do specify MF_CHECK

#TODO: Add 'check' as fs_mgr_flags with data partition.
# Currently we dont have e2fsck compiled. So fs check would failed.

#<src>                                                 <mnt_point>               <type>  <mnt_flags and options>                            <fs_mgr_flags>
/dev/block/by-name/system                               /system                   ext4    ro,barrier=1,discard                                 wait,slotselect,avb,first_stage_mount
/dev/block/by-name/vendor                               /vendor                   ext4    ro,barrier=1,discard                                 wait,slotselect,avb,first_stage_mount
/dev/block/bootdevice/by-name/op2                       /mnt/vendor/op2           ext4    noatime,nosuid,nodev,barrier=1,data=ordered          wait,check
/dev/block/bootdevice/by-name/userdata                  /data                     ext4    noatime,nosuid,nodev,barrier=1,noauto_da_alloc,discard wait,check,fileencryption=ice,quota,reservedsize=512M
/dev/block/bootdevice/by-name/modem                     /vendor/firmware_mnt      vfat    ro,shortname=lower,uid=1000,gid=1000,dmask=227,fmask=337,context=u:object_r:firmware_file:s0 wait,slotselect
/dev/block/bootdevice/by-name/dsp                       /vendor/dsp               ext4    ro,nosuid,nodev,barrier=1                            wait,slotselect
/dev/block/bootdevice/by-name/persist                   /mnt/vendor/persist       ext4    noatime,nosuid,nodev,barrier=1                       wait
/dev/block/bootdevice/by-name/bluetooth                 /vendor/bt_firmware       vfat    ro,shortname=lower,uid=1002,gid=3002,dmask=227,fmask=337,context=u:object_r:bt_firmware_file:s0 wait,slotselect
/devices/platform/soc/a600000.ssusb/a600000.dwc3/xhci-hcd.0.auto*     /storage/usbotg    vfat    nosuid,nodev         wait,voldmanaged=usbotg:auto
# Need to have this entry in here even though the mount point itself is no longer needed.
# The update_engine code looks for this entry in order to determine the boot device address
# and fails if it does not find it.
/dev/block/bootdevice/by-name/misc                      /misc              emmc    defaults                                             defaults
/dev/block/zram0                                        none               swap    defaults                                             zramsize=1073741824
`)

	newdata := magiskboot.PatchEncryption(tdata)
	if len(newdata) == len(tdata) {
		t.Fatal("Failed, size still equal")
	}

	t.Log(string(newdata))

}
