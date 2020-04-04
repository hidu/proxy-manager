#!/bin/bash
cd $(dirname $0)
cd ../

if [ -z "$1" ];then
    bash build.sh
    bash build.sh windows
fi



cd dest
################################################

if [ -d conf ];then
  rm -rf conf
fi

rm -rf data/*
mkdir conf
cp ../conf/proxy.conf conf/proxy.conf
echo -e "name:admin psw:psw is_admin:admin">conf/users
echo -e "#proxy=http://127.0.0.1:8080">conf/pool.conf

t=$(date +"%Y%m%d%H")

rm proxy_man_*.tar.gz proxy_man_*.zip

################################################
target_linux="proxy_man_linux_$t.tar.gz"

mkdir linux
cp proxy_man ../script/proxy_control.sh linux/
cp -r conf linux/conf


dir_new="proxy_man"
if [ -d $dir_new ];then
  rm -rf $dir_new
fi

mv linux $dir_new
tar -czvf $target_linux $dir_new

rm -rf  $dir_new

################################################
target_windows="proxy_man_windows_$t.zip"


mkdir windows
cp proxy_man.exe windows
cp ../script/windows_run.bat windows/start.bat 
cp -r conf windows/conf


mv windows $dir_new
zip -r $target_windows $dir_new

rm -rf  $dir_new conf



