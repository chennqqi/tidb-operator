#!/bin/bash

# docker private registries
registries=10.209.224.13:10500

# set -x
if (( $EUID != 0 )); then
    echo "Please run as root"
    exit
fi

images=(gcr.io/google_containers/kube-apiserver-amd64:v1.6.0 gcr.io/google_containers/kube-controller-manager-amd64:v1.6.0 gcr.io/google_containers/kube-scheduler-amd64:v1.6.0  gcr.io/google_containers/kube-proxy-amd64:v1.6.0 gcr.io/google_containers/etcd-amd64:3.0.17 gcr.io/google_containers/pause-amd64:3.0 gcr.io/google_containers/k8s-dns-sidecar-amd64:1.14.1 gcr.io/google_containers/k8s-dns-kube-dns-amd64:1.14.1  gcr.io/google_containers/k8s-dns-dnsmasq-nanny-amd64:1.14.1 gcr.io/google_containers/kubernetes-dashboard-amd64:v1.6.0 weaveworks/weaveexec:1.9.8)

# Install docker and kubernetes
k8sRpmDir=$1

if [ ! -d "$k8sRpmDir" ]; then
  echo "Please specify k8s rpm package directory: ./instal.sh ."
  exit 1
fi

# Check k8s rpm
# http://stackoverflow.com/questions/15305556/shell-script-to-check-if-file-exists
(
  shopt -s nullglob
  files=($k8sRpmDir/*kube*.rpm)
  if [[ "${#files[@]}" -eq 4 ]] ; then
    echo "Checking kubenetes rpm...ok"
  else
    echo "Checking kubenetes rpm...fail"
    exit 1
  fi
)

# clear
kubeadm reset
# Delete all containers
docker rm -f $(docker ps -a -q)
# Delete all images
docker rmi -f $(docker images -q)
echo "Remove all images if exist...ok"

# Erase old k8s
rpm -e kubectl kubelet kubeadm kubernetes-cni
yum remove -y kubectl kubelet kubeadm kubernetes-cni

# Remove old version docker
yum remove -y ebtables docker docker-common container-selinux docker-selinux docker-engine socat
echo "Cleaning old...ok"

# Upgrade
# sudo yum update -y && yum upgrade -y
echo "Update centos...ok"

# Disabling SELinux by running setenforce 0 is required in order to allow containers to access the host filesystem
setenforce 0

# Docker v1.12 is recommended
# Display details about a docker
yum info docker-engine.x86_64 
yum install -y ebtables docker-engine-1.12.6 ntpdate
echo "Inatall docker v1.12.6...ok"

yum install -y socat
rpm -ivh $k8sRpmDir/*kube*.rpm
echo "Inatall kubernetes...ok"

set -e

# Set access to the docker registry protocol: https -> http
if [ ! -d "/etc/docker" ]; then
  sudo mkdir /etc/docker
fi
tee > /etc/docker/daemon.json <<- EOF
{ "insecure-registries":["$registries"] }
EOF

# start docker
systemctl enable docker && sudo systemctl start docker
echo "Start docker...ok"

# Pull the base image of kubernetes 
for imageName in ${images[@]} ; do
  docker pull $registries/$imageName
  docker tag $registries/$imageName $imageName
  docker rmi $registries/$imageName
done
echo "Pull kubernetes images...ok"

# initialize
systemctl enable kubelet && sudo systemctl start kubelet
echo "Reset k8s and start kubelet...ok"

echo 1 > /proc/sys/net/bridge/bridge-nf-call-iptables

# check SELinux
echo $(sestatus)
# vi /etc/sysconfig/selinux
# SELINUX=disabled
# reboot

tee > /etc/profile.d/k8s.sh <<- EOF
alias kubectl='kubectl --server=127.0.0.1:10218'
EOF

echo "Sync os time"
# sync system time: ntp.api.bz is china
ntpdate -u  10.209.100.2
# write system time to CMOS
clock -w

echo "Install kubernets...finished"