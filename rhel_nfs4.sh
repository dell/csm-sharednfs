#!/bin/sh
install_opts="--disablerepo 'pgdg*'"

echo "enter server IP"
read ip
echo "ip $ip ... continue?"
read x

echo "install nfs-utils"
dnf install nfs-utils $install_opts

if [ ! -e /etc/nfs.conf.orig ];
then
	echo "backing up /etc/nfs.conf"
	cp /etc/nfs.conf /etc/nfs.conf.orig
fi


echo "copying nfs.conf"
copy cp rhel_nfs.conf /etc/nfs.conf
cp rhel_nfs.conf /etc/nfs.conf

echo "disabling v3 services"
echo systemctl mask --now rpc-statd.service rpcbind.service rpcbind.socket
systemctl mask --now rpc-statd.service rpcbind.service rpcbind.socket

"configuring nfs-mount.service.d"
echo mkdir /etc/systemd/nfs-mount.service.d
mkdir /etc/systemd/nfs-mount.service.d
cat >/etc/systemd/nfs-mount.service.d/v4only.conf <<END
[Service]
ExecStart=
ExecStart=/usr/sbin/rpc.mountd --no-tcp --no-udp
END

echo systemctl daemon-reload
systemctl daemon-reload
echo systemctl restart nfs-mountd
systemctl restart nfs-mountd

echo "should see nfsd processes: "
ps axl | grep nfsd
echo "... done

mkdir /nfs
mkdir /nfs/exports

echo "opening firewall"
echo firewall-cmd --permanent --add-service nfs
firewall-cmd --permanent --add-service nfs
echo firewall-cmd --reload
firewall-cmd --reload

echo "setting test export"
echo mkdir /nfs/exports
mkdir /nfs/exports
echo chgrp users /nfs/exports
echo chgrp users /nfs/exports
chgrp users /nfs/exports
echo chmod 02770 /nfs/exports
chmod 02770 /nfs/exports
echo mkdir /nfs/exports/hello
mkdir /nfs/exports/hello
echochgrp users /nfs/exports/hello
chgrp users /nfs/exports/hello
echo chmod 02777 /nfs/exports/hello
chmod 02777 /nfs/exports/hello
echo "hello world" > /nfs/exports/hello/world

echo "updating /etc/exports"
echo "/nfs/exports/hello/  $ip/24(rw)" >> /etc/exports
cat /etc/exports
systemctl restart nfs.mountd

echo "check that you can mount /nfs/exports/hello with\nmkdir hello && mount -t nfs4 $ip:/exports/hello /hello"
