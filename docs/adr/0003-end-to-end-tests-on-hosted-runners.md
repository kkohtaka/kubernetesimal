# 0003. End-to-end tests on GitHub-hosted runners

- Status: accepted
- Date: 2026-11-25

## Context

The operator builds etcd clusters out of KubeVirt virtual machines. envtest can show that the controllers create the
right objects, but not that a virtual machine boots, that etcd starts in it, or that the operator can reach it. That
needs a cluster with KubeVirt, and KubeVirt needs hardware virtualization to run virtual machines at a usable speed.

CI runs on GitHub-hosted runners, which are virtual machines themselves. GitHub says that nested virtualization is
technically possible on them but not officially supported. Whether end-to-end tests can run there had to be measured.

### Measurements

Two throwaway workflows were run on standard `ubuntu-24.04` runners, six runs in total. Every runner had four cores
of an Intel Xeon Platinum 8573C, 15 GiB of memory, the `vmx` CPU flag, and a `/dev/kvm` device that only the `kvm`
group could open.

The first workflow started a stock CirrOS virtual machine on kind with KubeVirt v1.10.0:

- With emulation disabled, the virtual machine ran with the libvirt domain type `kvm`.
- With `useEmulation` enabled, it ran with the domain type `qemu` on the same kind of host. KubeVirt does not hand
  `/dev/kvm` to virtual machines at all when that option is set.

The second workflow deployed the operator and created `Etcd` objects backed by the Fedora 37 cloud image:

| Step | Time |
|---|---|
| Create the kind cluster and install Calico | about 40 s |
| Build the operator image and load it into the cluster | 72 to 82 s |
| Install KubeVirt, the Containerized Data Importer, and cert-manager | 106 to 120 s |
| Deploy the operator | 13 s |
| Import the Fedora image | 32 to 76 s |
| First etcd node ready, counted from creating the `Etcd` object | 51 s |
| Two more etcd nodes ready, started together | 53 s |

The Fedora guest finished booting, including cloud-init, about 13 seconds after its virtual machine started. With
three etcd virtual machines running, the runner used 5.4 GiB of its memory, and each virtual machine about 580 MiB.
The whole job took seven and a half minutes.

The runs also showed what does not work:

- With kindnet, the default network plugin of kind, the operator could not reach the SSH port of its virtual
  machines. The operator attaches virtual machines with KubeVirt's bridge binding, which hands the Pod's address to
  the guest together with its `/24` mask. The guest then looks for other Pods on its own link, and kindnet connects
  each Pod to the node with a point-to-point link instead. With Calico, which gives a Pod a host route only, the
  same test passed.
- The `Etcd` object never became ready. Its Service is backed by an EndpointSlice that lists the cluster addresses
  of the per-node Services, and Kubernetes does not route a Service to another Service's cluster address. Members
  join through the same Service, so a cluster of more than one member could not form. The three etcd nodes above
  were three clusters of one member each.
- The manifests in `config/default` could not be deployed as they are. They reference a metrics proxy image that is
  no longer published, limit the manager to 30Mi of memory, and send traces to a collector that does not exist.

## Options

1. **GitHub-hosted runners with KVM.** Free for a public repository, no infrastructure to keep running, and measured
   to be fast enough. Not guaranteed by GitHub.
2. **GitHub-hosted runners with software emulation.** Works wherever a runner works. The speed with a realistic
   guest was not measured, because KVM turned out to be available.
3. **Third-party hosted runners that offer nested virtualization.** Guaranteed, at a cost and with another vendor
   in the build.
4. **A self-hosted runner.** Full control, and a machine to maintain and to secure against pull requests.

## Decision

Option 1. End-to-end tests run on standard GitHub-hosted runners, on a kind cluster with KubeVirt, using KVM.

- They run for every pull request. Setting up the environment takes about five minutes, which is acceptable.
- The cluster uses Calico instead of kindnet for as long as the operator uses the bridge binding.
- Emulation stays disabled. If a runner has no usable KVM, the tests fail with a message that says so, instead of
  silently running an order of magnitude slower or timing out somewhere else.
- The versions of kind, Kubernetes, Calico, KubeVirt, the Containerized Data Importer, and cert-manager are pinned,
  and updated like other dependencies. The measurements used whatever was latest.

If hosted runners stop offering KVM, option 3 is the fallback, because it changes only the `runs-on` line.

## Consequences

- No infrastructure is needed for end-to-end tests, and M3 and M4 have headroom: three etcd virtual machines take a
  third of a runner's memory.
- The tests depend on something GitHub does not promise. Six runs are evidence, not a guarantee, and a change on
  GitHub's side would stop all end-to-end tests at once.
- Real end-to-end tests cannot pass before the operator is fixed. The EndpointSlice of the `Etcd` Service has to
  point at addresses that Kubernetes routes, and `config/default` has to be deployable. Both are behavior changes
  that go through a specification in M1.
- Whether virtual machines keep the bridge binding or move to the masquerade binding, which KubeVirt recommends for
  the Pod network and which does not depend on the network plugin, is a decision for the same specification. It
  decides whether the tests keep needing Calico.
- The measurement workflows live on the `claude/e2e-probe` branch. They are experiments. The end-to-end tests of M1
  are built from them, with assertions instead of timings.
