/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package kvm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/pkg/errors"
	pkgdrivers "github.com/praveenkumar/machine-kvm2-driver/pkg/drivers"
)

const (
	qemusystem                = "qemu:///system"
	defaultPrivateNetworkName = "minikube-net"
	defaultNetworkName        = "default"
	defaultSSHUser            = "docker"
	isoFilename               = "boot2docker.iso"
)

type Driver struct {
	*drivers.BaseDriver
	*pkgdrivers.CommonDriver

	// How much memory, in MB, to allocate to the VM
	Memory int

	// How many cpus to allocate to the VM
	CPU int

	// The name of the default network
	Network string

	// The name of the private network
	PrivateNetwork string

	// The size of the disk to be created for the VM, in MB
	DiskSize int

	// The path of the disk .img
	DiskPath string

	// A file or network URI to fetch the minikube ISO
	Boot2DockerURL string

	// The location of the iso to boot from
	ISO string

	// The randomly generated MAC Address
	// If empty, a random MAC will be generated.
	MAC string
}

func NewDriver(hostName, storePath string) *Driver {
	return &Driver{
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
			StorePath:   storePath,
			SSHUser:     "docker",
		},
		CommonDriver: &pkgdrivers.CommonDriver{},
	}
}

func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		mcnflag.IntFlag{
			Name:  "kvm-memory",
			Usage: "Size of memory for host in MB",
			Value: 1024,
		},
		mcnflag.IntFlag{
			Name:  "kvm-disk-size",
			Usage: "Size of disk for host in MB",
			Value: 20000,
		},
		mcnflag.IntFlag{
			Name:  "kvm-cpu-count",
			Usage: "Number of CPUs",
			Value: 1,
		},
		mcnflag.StringFlag{
			Name:  "kvm-network",
			Usage: "Name of network to connect to",
			Value: "default",
		},
		mcnflag.StringFlag{
			EnvVar: "KVM_BOOT2DOCKER_URL",
			Name:   "kvm-boot2docker-url",
			Usage:  "The URL of the boot2docker image. Defaults to the latest available version",
			Value:  "",
		},
		mcnflag.StringFlag{
			Name:  "kvm-cache-mode",
			Usage: "Disk cache mode: default, none, writethrough, writeback, directsync, or unsafe",
			Value: "default",
		},
		mcnflag.StringFlag{
			Name:  "kvm-io-mode",
			Usage: "Disk IO mode: threads, native",
			Value: "threads",
		},
		mcnflag.StringFlag{
			EnvVar: "KVM_SSH_USER",
			Name:   "kvm-ssh-user",
			Usage:  "SSH username",
			Value:  defaultSSHUser,
		},
	}
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	log.Debugf("SetConfigFromFlags called")
	d.Memory = flags.Int("kvm-memory")
	d.DiskSize = flags.Int("kvm-disk-size")
	d.CPU = flags.Int("kvm-cpu-count")
	d.Network = flags.String("kvm-network")
	d.Boot2DockerURL = flags.String("kvm-boot2docker-url")
	d.ISO = d.ResolveStorePath(isoFilename)
	d.SSHUser = flags.String("kvm-ssh-user")
	d.SSHPort = 22
	d.DiskPath = d.ResolveStorePath(fmt.Sprintf("%s.img", d.MachineName))
	return nil
}

func (d *Driver) PreCommandCheck() error {
	conn, err := getConnection()
	if err != nil {
		return errors.Wrap(err, "Error connecting to libvirt socket.  Have you added yourself to the libvirtd group?")
	}
	libVersion, err := conn.GetLibVersion()
	if err != nil {
		return errors.Wrap(err, "getting libvirt version")
	}
	log.Debugf("Using libvirt version %d", libVersion)

	return nil
}

func (d *Driver) GetURL() (string, error) {
	if err := d.PreCommandCheck(); err != nil {
		return "", errors.Wrap(err, "getting URL, precheck failed")
	}

	ip, err := d.GetIP()
	if err != nil {
		return "", errors.Wrap(err, "getting URL, could not get IP")
	}
	if ip == "" {
		return "", nil
	}

	return fmt.Sprintf("tcp://%s:2376", ip), nil
}

func (d *Driver) GetState() (state.State, error) {
	dom, conn, err := d.getDomain()
	if err != nil {
		return state.None, errors.Wrap(err, "getting connection")
	}
	defer closeDomain(dom, conn)

	libvirtState, _, err := dom.GetState() // state, reason, error
	if err != nil {
		return state.None, errors.Wrap(err, "getting domain state")
	}

	// Possible States:
	//
	// VIR_DOMAIN_NOSTATE no state
	// VIR_DOMAIN_RUNNING the domain is running
	// VIR_DOMAIN_BLOCKED the domain is blocked on resource
	// VIR_DOMAIN_PAUSED the domain is paused by user
	// VIR_DOMAIN_SHUTDOWN the domain is being shut down
	// VIR_DOMAIN_SHUTOFF the domain is shut off
	// VIR_DOMAIN_CRASHED the domain is crashed
	// VIR_DOMAIN_PMSUSPENDED the domain is suspended by guest power management
	// VIR_DOMAIN_LAST this enum value will increase over time as new events are added to the libvirt API. It reflects the last state supported by this version of the libvirt API.

	switch libvirtState {
	// DOMAIN_SHUTDOWN technically means the VM is still running, but in the
	// process of being shutdown, so we return state.Running
	case libvirt.DOMAIN_RUNNING, libvirt.DOMAIN_SHUTDOWN:
		return state.Running, nil
	case libvirt.DOMAIN_BLOCKED, libvirt.DOMAIN_CRASHED:
		return state.Error, nil
	case libvirt.DOMAIN_PAUSED:
		return state.Paused, nil
	case libvirt.DOMAIN_SHUTOFF:
		return state.Stopped, nil
	case libvirt.DOMAIN_PMSUSPENDED:
		return state.Saved, nil
	case libvirt.DOMAIN_NOSTATE:
		return state.None, nil
	default:
		return state.None, nil
	}
}

func (d *Driver) GetIP() (string, error) {
	s, err := d.GetState()
	if err != nil {
		return "", errors.Wrap(err, "machine in unknown state")
	}
	if s != state.Running {
		return "", errors.New("host is not running")
	}
	ip, err := d.lookupIP()
	if err != nil {
		return "", errors.Wrap(err, "getting IP")
	}

	return ip, nil
}

func (d *Driver) GetMachineName() string {
	return d.MachineName
}

func (d *Driver) GetSSHHostname() (string, error) {
	return d.GetIP()
}

func (d *Driver) GetSSHKeyPath() string {
	return d.ResolveStorePath("id_rsa")
}

func (d *Driver) GetSSHPort() (int, error) {
	if d.SSHPort == 0 {
		d.SSHPort = 22
	}

	return d.SSHPort, nil
}

func (d *Driver) GetSSHUsername() string {
	if d.SSHUser == "" {
		d.SSHUser = "docker"
	}

	return d.SSHUser
}

func (d *Driver) DriverName() string {
	return "kvm2"
}

func (d *Driver) Kill() error {
	dom, conn, err := d.getDomain()
	if err != nil {
		return errors.Wrap(err, "getting connection")
	}
	defer closeDomain(dom, conn)

	return dom.Destroy()
}

func (d *Driver) Restart() error {
	return pkgdrivers.Restart(d)
}

func (d *Driver) Start() error {
	log.Info("Getting domain xml...")
	dom, conn, err := d.getDomain()
	if err != nil {
		return errors.Wrap(err, "getting connection")
	}
	defer closeDomain(dom, conn)

	log.Info("Creating domain...")
	if err := dom.Create(); err != nil {
		return errors.Wrap(err, "Error creating VM")
	}

	log.Info("Waiting to get IP...")
	for i := 0; i <= 40; i++ {
		ip, err := d.GetIP()
		if err != nil {
			return errors.Wrap(err, "getting ip during machine start")
		}
		if ip == "" {
			log.Debugf("Waiting for machine to come up %d/%d", i, 40)
			time.Sleep(3 * time.Second)
			continue
		}

		if ip != "" {
			log.Infof("Found IP for machine: %s", ip)
			d.IPAddress = ip
			break
		}
	}

	if d.IPAddress == "" {
		return errors.New("Machine didn't return an IP after 120 seconds")
	}

	log.Info("Waiting for SSH to be available...")
	if err := drivers.WaitForSSH(d); err != nil {
		d.IPAddress = ""
		return errors.Wrap(err, "SSH not available after waiting")
	}

	return nil
}

func (d *Driver) Create() error {
	log.Info("Creating machine...")
	log.Info("Creating network...")
	err := d.createNetwork()
	if err != nil {
		return errors.Wrap(err, "creating network")
	}

	log.Info("Setting up minikube home directory...")
	if err := os.MkdirAll(d.ResolveStorePath("."), 0755); err != nil {
		return errors.Wrap(err, "Error making store path directory")
	}

	for dir := d.ResolveStorePath("."); dir != "/"; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&0011 != 1 {
			log.Debugf("Setting executable bit set on %s", dir)
			mode |= 0011
			os.Chmod(dir, mode)
		}
	}

	log.Info("Building disk image...")
	if err = pkgdrivers.MakeDiskImage(d.BaseDriver, d.Boot2DockerURL, d.DiskSize); err != nil {
		return errors.Wrap(err, "Error creating disk")
	}

	log.Info("Creating domain...")
	dom, err := d.createDomain()
	if err != nil {
		return errors.Wrap(err, "creating domain")
	}
	defer dom.Free()

	log.Debug("Finished creating machine, now starting machine...")
	return d.Start()
}

func (d *Driver) Stop() error {
	d.IPAddress = ""
	s, err := d.GetState()
	if err != nil {
		return errors.Wrap(err, "getting state of VM")
	}

	if s != state.Stopped {
		dom, conn, err := d.getDomain()
		defer closeDomain(dom, conn)
		if err != nil {
			return errors.Wrap(err, "getting connection")
		}

		err = dom.Shutdown()
		if err != nil {
			return errors.Wrap(err, "stopping vm")
		}

		for i := 0; i < 60; i++ {
			s, err := d.GetState()
			if err != nil {
				return errors.Wrap(err, "Error getting state of VM")
			}
			if s == state.Stopped {
				return nil
			}
			log.Infof("Waiting for machine to stop %d/%d", i, 60)
			time.Sleep(1 * time.Second)
		}

	}

	return fmt.Errorf("Could not stop VM, current state %s", s.String())
}

func (d *Driver) Remove() error {
	log.Debug("Removing machine...")
	conn, err := getConnection()
	if err != nil {
		return errors.Wrap(err, "getting connection")
	}
	defer conn.Close()

	//Tear down network and disk if they exist
	log.Debug("Checking if the network needs to be deleted")
	network, err := conn.LookupNetworkByName(d.PrivateNetwork)
	if err != nil {
		log.Warn("Network %s does not exist, nothing to clean up...", d.PrivateNetwork)
	}
	if network != nil {
		log.Infof("Network %s exists, removing...", d.PrivateNetwork)
		network.Destroy()
		network.Undefine()
	}

	log.Debug("Checking if the domain needs to be deleted")
	dom, err := conn.LookupDomainByName(d.MachineName)
	if err != nil {
		log.Warn("Domain %s does not exist, nothing to clean up...", d.MachineName)
	}
	if dom != nil {
		log.Infof("Domain %s exists, removing...", d.MachineName)
		dom.Destroy()
		dom.Undefine()
	}

	return nil
}
