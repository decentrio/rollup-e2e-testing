FROM rust:1.87

RUN apt-get update

RUN apt-get install -y protobuf-compiler libclang-dev

WORKDIR /home
COPY . .

WORKDIR /home/dymension/libs/demo/user/
