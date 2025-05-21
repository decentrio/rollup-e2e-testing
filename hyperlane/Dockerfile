FROM node:20-alpine

WORKDIR /root

RUN apk add --update --no-cache git g++ make py3-pip jq

RUN yarn set version 4.5.1

RUN npm install -g @daniel.dymension.xyz/hyperlane-cli

