#!/usr/bin/env bash

while :
do
    read -p "please enter appId(required):" appId
    if [ ! -z $appId ];then
        break
    fi
done
while :
do
    read -p "please enter appSecret(required):" appSecret
    if [ ! -z $appSecret ];then
        break
    fi
done
read -p "please enter listen port(default:8990):" listenPort
if [ -z $listenPort ];then
    listenPort="8990"
fi

echo 'input any key go on,or control+c over'
read

echo 'docker build'
docker build -t wx_gateway .
echo 'docker run'
docker run -d --restart=always --name wx_gateway -p $listenPort:8990 -e APP_ID=$appId -e APP_SECRET=$appSecret wx_gateway

echo 'all finish'
