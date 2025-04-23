#!/bin/bash
set -e
docker build -t cnlidong/go-socks5-server . --platform linux/amd64 

docker push cnlidong/go-socks5-serverdo