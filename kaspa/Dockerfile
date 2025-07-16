FROM rust:1.87

RUN apt-get update

RUN apt-get install -y protobuf-compiler libclang-dev

WORKDIR /home
COPY . .

WORKDIR /home/dymension/libs/kaspa/demo/user/

RUN cargo run validator-with-escrow

WORKDIR /home/rust/main/

RUN cargo build --release --bin validator