# Native Listener Independent Review

## Findings

An independent read-only review found that the first implementation combined
the correct AF_PACKET wildcard index with `SOCK_RAW` and fixed Ethernet
offsets. That would reject packets from interfaces with different link headers,
including loopback and TUN-style devices. It also made the BPF path inconsistent
with the parser's previous VLAN handling.

The review also identified missing IPv4 total-length validation and incomplete
negative coverage for fragmentation and malformed headers.

## Resolution

- Switched AF_PACKET capture from `SOCK_RAW` to cooked `SOCK_DGRAM`.
- Moved BPF and parser offsets to the normalized IPv4 header.
- Added IPv4 total-length validation in the parser.
- Added tests for fragment offset, `MF`, `DF`, invalid IP version, invalid IHL,
  IPv4 options, destination IP, and destination port.
- Kept IPv6 explicitly unsupported and rejected during configuration loading.

## Residual verification limit

The current environment has neither root nor `CAP_NET_RAW`, so the real socket
cannot be opened here. BPF bytecode is assembled and executed in `bpf.VM`; a
live AF_PACKET smoke test remains an environment-level verification step.
