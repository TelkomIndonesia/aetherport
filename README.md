# Aetherport

Layer 4 proxy utilizing WebRTC which enables port-forwarding without dedicated allocation of ingress IP and port.

## Use Cases

- access self-hosted services deployed on your home from any network connected to Internet.
- quick sharing of services being developed in your laptop to peer developers on another network.
- securely expose control plane service to data plane services on another data center or DMZ.
- ...

## Install

The binary are distributed as a single executable which are available from the [release page](./releases) and as Docker image which can be downloaded from the [package page](./pkgs/container/aetherport).

```bash
./aetherport --help
```

```bash
docker run --rm ghcr.io/telkomindonesia/aetherport --help
```

## Run

### Quick start without signaling server

1. On the sender side which will run an egress proxy, run:

   ```bash
   aetherport --forward 0.0.0.0:80:0.0.0.0:8080`
   ```

   This will forward connection to `0.0.0.0:80` on the sender side to `0.0.0.0:8080` on the receiver site.

1. `aetherport` will display an 'offer' text that should be copied to the receiver.

1. On the receiver side which will run an ingress proxy,

    ```bash
    aetherport --allow 0.0.0.0:8080
    ```

    This will allow connection to be forwarded to `0.0.0.0:8080.

1. Copy paste the 'offer' text displayed in step 2 to the prompt on receiver side and press `<enter>`.

1. `aetherport` will display an 'answer' text that should be copied to the sender.

1. Copy paste 'anwer' text displayed in step 5 to the prompt on sender side and press `<enter>`.

At the end, a connection will be established that will forward any traffic received on `0.0.0.0:80` on the sender side to `0.0.0.0:8080` on the receiver side.

## Run with signalling server

1. Run the aetherlight signaling server on machine reachable by all participating node. The server will be listening on `http://0.0.0.0:8080` by default.

    ```bash
    ./aetherport signal
    ```

1. Generate certificate for each node.

    ```bash
    ./aetherport cert generate --name node1
    ./aetherport cert generate --name node2
    ...
    ```

1. Distribute the node private key, certificate, and ca-certificate file to each node.

1. On the node that will run an ingress proxy (for example to expose a service in 0.0.0.0:80), run the following:

    ```bash
    ./aetherport \
        --key '<path to key file>' \
        --cert '<path to certificate file>' \
        --cacert '<path to ca certificate file>' \
        --allow '0.0.0.0:80' \
        --signal-type 'aetherlight' \
        --aetherlight-base-url '<base url where aetherlight is available, e.g. http://0.0.0.0:8080>'
    ```

    Note the url that will be echoed in the terminal. The url will be used on any node that will connect to this node.

1. On the node that will run the egress proxy and try to connect to node above, run:

    ```bash
    ./aetherport \
        --key '<path to key file>' \
        --cert '<path to certificate file>' \
        --cacert '<path to ca certificate file>' \
        --forward '0.0.0.0:80800.0.0.0:80' \
        --signal-type 'aetherlight' \
        --aetherlight-ingress-url '<base url where aetherlight is available, e.g. http://0.0.0.0:8080>/ingresses/<ingress ID from the previous command>'
    ```

Note that a node can run the aetherport command to act both as ingress or egress proxy at the same time.

## Roadmap

- [ ] UDP forwarding.
- [ ] TCP to stdio forwarding.
- [ ] Socks5 proxy on the sender side.
- [ ] Equal or better performance with ssh port forwarding.
- [ ] Fine-grained access control on the receiver side.
- [ ] Application protocol filter (e.g. HTTP).
