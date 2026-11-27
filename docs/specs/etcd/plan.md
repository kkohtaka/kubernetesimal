# Plan for the etcd specification

This plan belongs to the [specification of etcd clusters](spec.md). It says how the requirements are going to be
met and tested. It is based on a reading of all controller code, which was not executed, and on the experiments of
[ADR 0003](../../adr/0003-end-to-end-tests-on-hosted-runners.md).

## Decisions

These were decided while drafting. Each one is a choice between behaviors that a user can observe, so each one is
the owner's to overrule before the specification is approved.

1. **Zero replicas are rejected (ETCD-005).** A cluster without members has lost its data, and deleting the `Etcd`
   object is the way to say that. Today zero is accepted and reported as an error.
2. **Even numbers of replicas are accepted.** They are a poor choice for etcd, because they add no fault tolerance
   over the next lower odd number, but they occur on the way from one odd size to another.
3. **Members are added and removed one at a time (ETCD-025, ETCD-051).** Adding two members to a cluster of one at
   the same moment leaves it without a quorum until the first of them has started. One at a time is slower and safe.
4. **A replacement adds before it removes (ETCD-061).** This is what the code does today for up to three members.
   For four and more it currently lets one member be unavailable. The requirement makes it the same for every size.
5. **Deleting an `Etcd` object does not ask its members to leave (ETCD-071).** The cluster is going away as a whole,
   so removing members one by one gains nothing and is the reason deletion can hang today.
6. **A member is removed through the cluster, not from inside its own virtual machine (ETCD-052, ETCD-053).** The
   operator calls the member API of etcd on a remaining member. The command inside the virtual machine stays as a
   best effort to stop etcd there. A member that is broken can then be removed, which is the case that matters.
7. **Detecting a failure may take five minutes (ETCD-048).** A healthy member is probed every three minutes today.
   Making that faster is cheap, and is left for when someone needs it.
8. **The phases keep their current meaning (ETCD-043 to ETCD-046).** In particular the phase is `Creating`, not
   `Running`, while a healthy cluster is being resized or replaced. That is odd, and renaming phases is an
   incompatible change that is better made together with the move to standard conditions in M2.
9. **Members only accept clients with a certificate (ETCD-015).** This is what etcdadm is believed to configure. It
   was not verified. If the end-to-end test shows otherwise, the requirement stays and the implementation is fixed.

Not fixed in M1, by decision of the owner: everything under "Known limitations" in the specification, and the
network binding of the virtual machines, which stays the bridge binding.

## Design

### Validation and defaults (ETCD-001 to ETCD-006)

Move both from the webhook into the schema of the `Etcd` kind:

- `+kubebuilder:default` for `spec.version` and `spec.replicas`.
- `+kubebuilder:validation:Minimum=1` for `spec.replicas`.
- A pattern for `spec.version` that accepts what the semantic version parser accepts.
- A local type for the image reference, or a CEL rule, that requires a non-empty `name`.

The webhook then has nothing left to do and is removed, together with its certificate, its manifests, and its test
suite. `make api-check` reports the tightened validation, which is recorded in `docs/api-breaking-changes.md`. The
controller stops dereferencing the two pointers without a check.

### The cluster Service (ETCD-020, ETCD-030 to ETCD-034)

The EndpointSlice of the cluster Service lists the address of each member's virtual machine, taken from
`status.interfaces` of its VirtualMachineInstance, instead of the cluster address of the member's Service. With the
bridge binding that address is routable in the Pod network.

An endpoint is `ready` when the member is ready and not being removed, as today. Members are listed in the namespace
of the `Etcd` object only.

The alternative, a Service with a selector that matches the Pods of the virtual machines, would drop the
hand-written EndpointSlice. It makes readiness follow the readiness probe of the virtual machine, which checks SSH
and not etcd, so it would send clients to members where etcd is not running. It is not chosen.

The same lookup of members, restricted to the namespace, is used by the probe of ETCD-042.

### Reconciling when members change (ETCD-047, ETCD-048)

The `EtcdNodeDeployment` gets a controller owner reference to its `Etcd` object, so that the existing watch fires.
The watch on `EtcdNode` objects, which never fires because they are owned two levels down, is replaced by a mapping
from the labels of an `EtcdNode` to its `Etcd` object.

Every wait that returns without a delay today gets one. The affected places are the waits for Secrets, Services, and
cluster addresses in `controllers/etcd` and `controllers/etcdnode`. This is ETCD-026 for the password Secret, and
removes a whole class of silent stalls for the rest.

### One member at a time (ETCD-025, ETCD-050, ETCD-051, ETCD-061)

The `EtcdNodeSet` controller creates at most one `EtcdNode` per pass and only when every node it already has is
ready, and deletes at most one per pass and only when no node is being deleted. The `EtcdNodeDeployment` controller
fixes the surge at one and the unavailability at zero, whatever the size, and its `rollingUpdate` field is removed.

This turns the two controllers from a port of Deployment and ReplicaSet, with parallel batches, into something much
simpler. Code that only served the general case goes: percentage resolution, the ranking of nodes by co-location,
proportional scaling leftovers, and the annotations that nothing reads.

### Removing a member (ETCD-052, ETCD-053)

Finalizing an `EtcdNode` first removes the member through the cluster: the operator connects to the cluster Service
with the client certificate, finds the member by its name, and calls the member API to remove it. Not finding it
counts as done. Then it tries the leave script inside the virtual machine, with a timeout, and ignores its failure.
Then it deletes the virtual machine.

When the cluster cannot be reached at all, removal is retried. That can only last while the `Etcd` object exists,
see the next section.

### Deleting a cluster (ETCD-070, ETCD-071)

When its `Etcd` object is being deleted, an `EtcdNode` skips the removal of its member and deletes its virtual
machine at once. The `EtcdNode` learns this from an annotation that the `Etcd` controller puts on the
`EtcdNodeDeployment` before it deletes it, and that is passed down. The `Etcd` controller waits until no `EtcdNode`
of the cluster is left before it deletes the Secrets and removes its finalizer.

### Building from the latest specification (ETCD-063, ETCD-064)

The branch that scales the first member away when the specification changes before the first readiness is removed.
A changed template is simply applied. The first member is then replaced like any other, which ETCD-061 covers.

### Not crashing and not stalling (ETCD-080 to ETCD-082)

- The `Etcd` reconciler returns the status it had when creating or updating the members fails, instead of nil.
- `main.go` turns on the recovery of panics in reconcilers, as a second line of defense.
- SSH gets a connect timeout and a deadline for each command. The scripts get a timeout on the download.
- The schemas of the internal kinds get minimums where a negative number makes a controller hang or panic today.
- The errors that are swallowed in the two rollout controllers are returned.
- Each controller that talks to virtual machines gets more than one worker.

### Seams for tests

Two things the operator does cannot happen in envtest: it runs commands in a virtual machine over SSH, and it talks
to etcd. Both go behind small interfaces that the reconcilers receive, with the real implementations in `main.go`
and fakes in tests:

- A command runner: run a named script on a member, with a timeout.
- An etcd cluster client: health, member list, member status, member removal.

envtest also has no KubeVirt. The tests install the CRD of VirtualMachineInstance and play the part of KubeVirt by
writing the status of the objects the operator creates.

## How each requirement is tested

| Requirements | Test | Notes |
|---|---|---|
| ETCD-001 to ETCD-006 | envtest | The API server applies the schema. ETCD-006 holds because no webhook is involved. |
| ETCD-010 to ETCD-014 | envtest | The certificates are parsed and verified against the authority. |
| ETCD-015 | end-to-end | Needs a real etcd. |
| ETCD-020, ETCD-021 | end-to-end | Reads the member list and the version from the cluster. |
| ETCD-022 | envtest | Checks how the virtual machine references the claim. |
| ETCD-023, ETCD-024 | envtest | Checks the cloud-init data that the virtual machine is given. For ETCD-023 that is the password of the default user, which the preconditions of the specification say is `fedora`. Logging in is not tested. |
| ETCD-025 | envtest and end-to-end | envtest controls when a fake member becomes ready and counts the members. |
| ETCD-026 | envtest | |
| ETCD-030, ETCD-032, ETCD-033 | envtest | Checks the Service and the endpoints. |
| ETCD-031 | end-to-end | Connects from a Pod. |
| ETCD-034 | envtest | Two objects with one name in two namespaces. |
| ETCD-040 to ETCD-048 | envtest | The fake etcd client decides what the probes see. The time limits are checked with the real intervals. |
| ETCD-050 to ETCD-052 | envtest and end-to-end | envtest checks the order of calls. End-to-end checks the member list. |
| ETCD-053 | envtest and end-to-end | End-to-end stops a virtual machine and scales down. |
| ETCD-060, ETCD-061, ETCD-063, ETCD-064 | envtest | Counts members and ready members throughout. |
| ETCD-060, ETCD-062 | end-to-end | Writes a key, changes the version, reads the key. |
| ETCD-070 | envtest and end-to-end | |
| ETCD-071 | envtest and end-to-end | envtest makes the fakes fail. End-to-end stops the virtual machines first. |
| ETCD-080 | envtest | Makes a create fail by putting an object with the same name and another owner in the way. |
| ETCD-081 | envtest | A command runner that blocks for one cluster. |
| ETCD-082 | envtest | Stops and starts the manager within the test. |

The end-to-end tests run as [ADR 0003](../../adr/0003-end-to-end-tests-on-hosted-runners.md) decides. ETCD-023's
login and the preconditions under "Background" are not verified automatically.

## Order of work

Each step is one pull request, leaves all tests passing, and comes with a change file where users can notice it.

1. **Seams.** Introduce the two interfaces and the envtest harness with the fake KubeVirt. No behavior changes.
   Label: `skip-changelog`.
2. **Tests for what should already hold.** ETCD-010 to ETCD-014, ETCD-022 to ETCD-024, ETCD-030, ETCD-033,
   ETCD-040, ETCD-043 to ETCD-046. Where one fails, the specification's table of differences was incomplete: report
   it, and fix it in its own step.
3. **Validation and defaults in the schema, and removal of the webhook.** ETCD-001 to ETCD-006.
4. **Not crashing.** ETCD-080, and the timeouts of ETCD-081.
5. **The cluster Service and namespaces.** ETCD-031, ETCD-032, ETCD-034. From here on an `Etcd` object can become
   ready, and end-to-end tests are possible.
6. **End-to-end tests for one member.** ETCD-015, ETCD-021, ETCD-031, with the suite of issue #292.
7. **Reconciling when members change, and waits that retry.** ETCD-026, ETCD-047, ETCD-048.
8. **One member at a time.** ETCD-025, ETCD-050, ETCD-051, ETCD-061, ETCD-063, ETCD-064, and end-to-end tests for
   three members, ETCD-020.
9. **Removing a member through the cluster.** ETCD-052, ETCD-053.
10. **Deleting a cluster.** ETCD-070, ETCD-071.
11. **Replacement end to end.** ETCD-060, ETCD-062, ETCD-082.
12. Set `status: implemented`.

Steps 3 and 4 do not depend on each other, and neither do 9 and 10.

## Risks

- **etcdadm.** Whether it sets up client certificate authentication, what addresses it advertises, and how its
  commands behave when repeated, is not known from this repository. Step 6 is where that shows. If it cannot do what
  ETCD-015 or ETCD-020 need, replacing it moves from M2 into M1.
- **The address of a virtual machine.** The design assumes that `status.interfaces` of a VirtualMachineInstance
  holds an address that Pods can reach, and that etcd listens on it. The experiments reached SSH this way with
  Calico. etcd on port 2379 was not tried.
- **The size of step 8.** It rewrites the core of two controllers. It is also where most of the code that was
  copied from upstream and never exercised goes away.
