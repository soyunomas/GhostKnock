# Native Listener Hardening

## Decision

This phase keeps the Linux daemon listener IPv4-only. IPv6 capture requires a
dual-stack EtherType policy, IPv6 extension-header parsing, equivalent BPF, and
end-to-end client address handling; mixing that work into the `any` and BPF fix
would increase the security-sensitive change substantially.

IPv6 `listener.listen_ip` values therefore fail during configuration loading.
They must not be accepted and then silently ignored.

## `interface: any`

Linux AF_PACKET uses `sockaddr_ll.sll_ifindex = 0` for all interfaces. The
`mdlayher/packet` API dereferences its `*net.Interface`, so passing `nil`
panics. A non-nil synthetic interface with index `0` preserves the kernel
wildcard semantics without forking the dependency.

The socket uses `SOCK_DGRAM`, not `SOCK_RAW`. Linux removes the physical link
header in this mode, so one IPv4 parser and BPF layout works across Ethernet,
loopback, and TUN-style interfaces; VLAN normalization is delegated to the
kernel. Assuming a fixed 14-byte Ethernet header would make `any` incomplete.

## Kernel BPF

`packet.Config.Filter` is attached before `bind(2)`. The filter accepts only:

- IPv4 UDP;
- non-fragmented packets;
- the configured destination IPv4, when present;
- the configured UDP destination port.

The BPF input starts at the IPv4 header because the socket is in cooked
datagram mode. The destination-port load uses the IPv4 IHL so packets with
IPv4 options are handled correctly. The existing Go parser remains a second
validation layer and also validates IPv4 total length.

## Verification boundary

Unit tests execute the assembled BPF in `bpf.VM`, so they require neither root
nor a live interface. Opening the real AF_PACKET socket still requires root or
`CAP_NET_RAW`.
