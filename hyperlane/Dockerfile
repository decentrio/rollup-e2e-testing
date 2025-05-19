FROM node:20-alpine

WORKDIR /hyperlane-monorepo

RUN apk add --update --no-cache git g++ make py3-pip jq

RUN yarn set version 4.5.1

COPY . .

RUN npm install -g @daniel.dymension.xyz/hyperlane-cli

