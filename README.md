This is developed using https://github.com/dhiltgen/docker-machine-kvm and https://github.com/kubernetes/minikube/tree/master/pkg/drivers/kvm

# docker-machine-kvm2
KVM2 driver for docker-machine

This driver leverages the new [plugin architecture](https://github.com/docker/machine/issues/1626) being
developed for Docker Machine.

# Quick start instructions

* Install `libvirt` and `qemu-kvm` on your system (e.g., `sudo apt-get install libvirt-bin qemu-kvm`)
    * Add yourself to the `libvirtd` group (may vary by linux distro) so you don't need to sudo
* Install [docker-machine](https://github.com/docker/machine/releases)
* Go to the
  [releases](https://github.com/dhiltgen/docker-machine-kvm/releases)
  page and download the docker-machine-driver-kvm binary, putting it
  in your PATH.
* You can now create virtual machines using this driver with
  `docker-machine create -d kvm myengine0`.

# Build from Source
```
$ yum install -y libvirt-devel curl git gcc  //CentOS,Fedora

$ apt-get install -y libvirt-dev curl git gcc //Ubuntu

$ make build
```

# Capabilities

## Images
By default `docker-machine-kvm` uses a [boot2docker.iso](https://github.com/boot2docker/boot2docker) as guest os for the kvm hypervisior. It's also possible to use every guest os image that is derived from [boot2docker.iso](https://github.com/boot2docker/boot2docker) as well.
For using another image use the `--kvm-boot2docker-url` parameter. 

## Dual Network

   * **eth1** - A host private network called **docker-machines** is automatically created to ensure we always have connectivity to the VMs.  The `docker-machine ip` command will always return this IP address which is only accessible from your local system.
   * **eth0** - You can specify any libvirt named network.  If you don't specify one, the "default" named network will be used.
        * If you have exotic networking topolgies (openvswitch, etc.), you can use `virsh edit mymachinename` after creation, modify the first network definition by hand, then reboot the VM for the changes to take effect.
        * Typically this would be your "public" network accessible from external systems
        * To retrieve the IP address of this network, you can run a command like the following:
        ```bash
        docker-machine ssh mymachinename "ip -one -4 addr show dev eth0|cut -f7 -d' '"
        ```

## Driver Parameters

Here are all currently driver parameters listed that you can use.

| Parameter     | Description| 
| ------------- | ------------- | 
| **--kvm-cpu-count**     | Sets the used CPU Cores for the KVM Machine. Defaults to `1` . | 
| **--kvm-disk-size**    | Sets the kvm machine Disk size in MB. Defaults to `20000` .      |  
| **--kvm-memory** | Sets the Memory of the kvm machine in MB. Defaults to `1024`.      | 
| **--kvm-network** | Sets the Network of the kvm machinee which it should connect to. Defaults to `default`.      |   
| **--kvm-boot2docker-url** | Sets the url from which host the image is loaded. By default it's not set.   |
| **--kvm-cache-mode** | Sets the caching mode of the kvm machine. Defaults to `default`.   |    
| **--kvm-io-mode-url** | Sets the disk io mode of the kvm machine. Defaults to `threads`.   |      

