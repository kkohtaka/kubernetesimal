---
status: approved
---

# etcd clusters

## Purpose

A user describes an etcd cluster with one `Etcd` object, and the operator builds it out of KubeVirt virtual machines,
keeps it at the described size and version, and removes it again. This specification says what the user can rely on.

The code that does this predates the specifications. This document writes down its behavior where that behavior is
sound, and says what it has to become where it is not. The differences are listed under
[Differences from the current behavior](#differences-from-the-current-behavior).

## Scope

The `Etcd` kind of the API group `kubernetesimal.kkohtaka.org`, version `v1alpha1`, and everything a user can observe
as a result of creating, changing, and deleting one: its status, the Secrets and the Service that belong to it, its
virtual machines, and the etcd cluster that runs in them.

### Out of scope

- **The kinds `EtcdNodeDeployment`, `EtcdNodeSet`, and `EtcdNode`.** They are how the operator is built, not an
  interface for users. Nothing here promises how they behave, and they may change or disappear. Users create `Etcd`
  objects only.
- **Installing the operator.** See the [installation specification](../install/spec.md).
- **Everything under [Known limitations](#known-limitations).** Those are weaknesses that this specification
  deliberately leaves as they are. Most of them are the subject of the next milestone.

## Background

### What an etcd cluster needs from its environment

These are preconditions, not requirements. When one does not hold, the behavior is unspecified.

- KubeVirt is installed, and the cluster can run virtual machines.
- The network plugin of the cluster delivers traffic between a Pod and a virtual machine that is attached to the Pod
  network with KubeVirt's bridge binding. Not every plugin does, see
  [ADR 0003](../../adr/0003-end-to-end-tests-on-hosted-runners.md).
- `spec.imagePersistentVolumeClaimRef` names a PersistentVolumeClaim in the namespace of the `Etcd` object that holds
  a bootable disk image for the `amd64` architecture. The image runs cloud-init and an SSH server, and has systemd
  and `curl`. Its default user, the one that cloud-init sets up and whose password and SSH keys a cloud-config sets
  when it names no user, is `fedora`, and that user may use `sudo` without a password.
- The virtual machines can download from `github.com`.

### Terms

- A **member** is one virtual machine that runs one etcd process which belongs to the cluster.
- A member is **ready** when its own etcd answers a health check.
- The **cluster Service** is the Service that has the name of the `Etcd` object.
- The **client certificate** is the certificate in the Secret `api-client-<name>`.

## Requirements

Unless a requirement says otherwise, "the cluster" is the etcd cluster of one `Etcd` object named `<name>`, and all
objects are in the namespace of that object.

### The API

- **ETCD-001**: An `Etcd` object that is created without `spec.version` gets the version `3.5.1`.
- **ETCD-002**: An `Etcd` object that is created without `spec.replicas` gets one replica.
- **ETCD-003**: Creating or updating an `Etcd` object is rejected when `spec.version` is not a semantic version of the
  form `MAJOR.MINOR.PATCH`, such as `3.5.6`. The error names the field `spec.version`.
- **ETCD-004**: Creating or updating an `Etcd` object is rejected when `spec.imagePersistentVolumeClaimRef.name` is
  missing or empty. The error names the field.
- **ETCD-005**: Creating or updating an `Etcd` object is rejected when `spec.replicas` is less than one. The error
  names the field `spec.replicas`.
- **ETCD-006**: The defaults of ETCD-001 and ETCD-002 and the rejections of ETCD-003 to ETCD-005 also apply while the
  operator is not running.

### Credentials

- **ETCD-010**: For an `Etcd` object the operator creates the Secret `ca-<name>` of the type `kubernetes.io/tls`. Its
  `tls.crt` is a self-signed certificate authority, and its `tls.key` is the matching private key.
- **ETCD-011**: The operator creates the Secret `api-client-<name>` of the type `kubernetes.io/tls`. Its `tls.crt` is
  a certificate that is signed by the certificate authority of ETCD-010 and valid for client authentication, and its
  `tls.key` is the matching private key.
- **ETCD-012**: The operator creates the Secret `ssh-keypair-<name>` of the type `kubernetes.io/ssh-auth`. Its
  `ssh-privatekey` is a private key in PEM format, and its `ssh-publickey` is the matching public key in the format
  of an `authorized_keys` line.
- **ETCD-013**: The status of the `Etcd` object names these Secrets and their keys in `caCertificateRef`,
  `caPrivateKeyRef`, `clientCertificateRef`, `clientPrivateKeyRef`, `sshPrivateKeyRef`, and `sshPublicKeyRef`.
- **ETCD-014**: Once the Secrets of ETCD-010 to ETCD-012 exist, the operator does not change their contents, however
  often the `Etcd` object is reconciled or changed.
- **ETCD-015**: A member accepts client connections only from clients that present a certificate signed by the
  certificate authority of ETCD-010. A connection with the client certificate of ETCD-011 is accepted, and one
  without a certificate is refused.

### Forming a cluster

- **ETCD-020**: For an `Etcd` object with `spec.replicas` of N, the operator brings up N members that form a single
  etcd cluster: every member reports the same cluster identifier, and the member list of the cluster has exactly N
  entries.
- **ETCD-021**: Every member runs the etcd version of `spec.version`.
- **ETCD-022**: The virtual machine of a member boots from a copy of the image in the PersistentVolumeClaim of
  `spec.imagePersistentVolumeClaimRef`. The operator does not write to that PersistentVolumeClaim, and several
  members use it at the same time.
- **ETCD-023**: When `spec.loginPasswordSecretKeySelector` is set, the user `fedora` can log in on the console of a
  member's virtual machine with the password in that Secret key. When it is not set, no password is set.
- **ETCD-024**: The SSH server of a member's virtual machine does not accept passwords, whether or not
  `spec.loginPasswordSecretKeySelector` is set.
- **ETCD-025**: Members are added to the cluster one at a time. The operator starts adding the next member only after
  the previous one is ready.
- **ETCD-026**: When the Secret that `spec.loginPasswordSecretKeySelector` names does not exist yet, no members are
  created. Within one minute after the Secret is created, the operator continues, without any change to the `Etcd`
  object.

### Reaching the cluster

- **ETCD-030**: The operator creates the cluster Service, a Service named `<name>` with a port named `etcd` on port
  2379.
- **ETCD-031**: A client in the Kubernetes cluster that connects to the cluster Service on port 2379 with the client
  certificate reaches a ready member.
- **ETCD-032**: A member that is not ready, or that is being removed, receives no new connections through the cluster
  Service.
- **ETCD-033**: `status.serviceRef` names the cluster Service.
- **ETCD-034**: Two `Etcd` objects with the same name in different namespaces do not affect each other. The cluster
  Service of each one reaches only its own members, and the status of each one describes only its own members.

### Status

- **ETCD-040**: `status.readyReplicas` is the number of members that are ready.
- **ETCD-041**: The condition `Ready` is `True` when a request for `/health` through the cluster Service, made with
  the client certificate, is answered with the HTTP status 200. Otherwise it is `False`.
- **ETCD-042**: The condition `MembersHealthy` is `True` when the member list of the cluster and the members that the
  operator runs are the same set, and every member in the list answers a status request. Otherwise it is `False`,
  and its message names a member that is missing, unexpected, or does not answer.
- **ETCD-043**: `status.phase` is `Deleting` once deletion of the `Etcd` object has been requested.
- **ETCD-044**: Otherwise `status.phase` is `Running` when the condition `Ready` is `True` and `status.readyReplicas`
  equals `spec.replicas`.
- **ETCD-045**: Otherwise `status.phase` is `Error` when the condition `Ready` is `False` and either has been `True`
  before or `status.readyReplicas` equals `spec.replicas`.
- **ETCD-046**: Otherwise `status.phase` is `Creating`.
- **ETCD-047**: When a member becomes ready, `status.readyReplicas`, `status.phase`, and the conditions show it
  within 30 seconds.
- **ETCD-048**: When a member stops being ready, `status.readyReplicas`, `status.phase`, and the conditions show it
  within five minutes.

### Changing the size

- **ETCD-050**: When `spec.replicas` is increased, the operator adds members as ETCD-025 says, until the cluster has
  that many. Members that exist are kept.
- **ETCD-051**: When `spec.replicas` is decreased, the operator removes members until the cluster has that many. It
  removes them one at a time, and starts removing the next only after the previous one is gone from the member list.
- **ETCD-052**: A member that is removed is taken out of the member list of the cluster before its virtual machine is
  deleted.
- **ETCD-053**: A member whose virtual machine does not answer, or no longer exists, can still be removed. It is taken
  out of the member list through the members that remain.

### Changing the version or the image

- **ETCD-060**: When `spec.version`, `spec.imagePersistentVolumeClaimRef`, or `spec.loginPasswordSecretKeySelector`
  changes, the operator replaces every member with a new one that is built from the changed specification. When it
  has finished, the cluster has `spec.replicas` members again, all of them new.
- **ETCD-061**: During the replacement the operator adds a new member and waits until it is ready before it removes
  an old one. The cluster never has fewer than `spec.replicas` ready members because of the replacement, and never
  more than `spec.replicas` plus one members.
- **ETCD-062**: Data that was written to the cluster before the replacement can be read from it afterwards.
- **ETCD-063**: When the specification changes again while a replacement is in progress, the cluster ends up with
  `spec.replicas` members that are all built from the latest specification.
- **ETCD-064**: When the specification changes before the cluster has become ready for the first time, the cluster is
  built from the latest specification.

### Deleting

- **ETCD-070**: When an `Etcd` object is deleted, the operator deletes the virtual machines of all members, the
  cluster Service, and the Secrets of ETCD-010 to ETCD-012. The `Etcd` object is gone only after the virtual machines
  are gone.
- **ETCD-071**: Deleting an `Etcd` object does not depend on its members. It completes when the virtual machines do
  not answer, when etcd in them is not running, and when the cluster has lost its quorum.

### Staying in operation

- **ETCD-080**: When the operator cannot create or update an object that belongs to an `Etcd` object, it tries again
  until it succeeds. The operator keeps running, and the status of the `Etcd` object keeps the values it had.
- **ETCD-081**: An `Etcd` object whose virtual machines do not answer does not hold up other `Etcd` objects. A
  cluster that is created while another one is in that state becomes ready as it would otherwise.
- **ETCD-082**: When the operator restarts, the clusters keep running, and no member is replaced or removed because of
  the restart.

## API changes

- `spec.replicas` gets the minimum 1 in the schema. It was 0. An object with zero replicas is no longer accepted.
- `spec.imagePersistentVolumeClaimRef.name` becomes required and must not be empty.
- `spec.version` and `spec.replicas` get their defaults from the schema instead of the webhook.
- `spec.version` gets a pattern for `MAJOR.MINOR.PATCH`. Versions with a pre-release or build suffix, such as
  `3.6.0-rc.1`, were accepted and no longer are.
- The admission webhook of the `Etcd` kind is removed, because the schema does everything it did. The operator then
  no longer needs cert-manager.
- The kinds `EtcdNodeDeployment`, `EtcdNodeSet`, and `EtcdNode` are declared internal. Their schemas change as the
  plan describes. Those changes are recorded as incompatible API changes, although no user is meant to depend on
  them.

No field is added to or removed from the `Etcd` kind.

## Permission changes

The operator needs no new permissions. Two of the permissions it has are unused and are removed: the probing
controllers only read Secrets and Services, and lose the verbs that write them.

## Differences from the current behavior

The requirements above describe the intended behavior. The code does not meet these today:

| Requirement | Today |
|---|---|
| ETCD-004 | The reference is checked on update but not on create. An object that is created with an empty reference can never be updated, not even by the operator. |
| ETCD-005 | Zero replicas are accepted, and the object then reports the phase `Error`. |
| ETCD-006 | Defaults and most checks come from a webhook inside the operator. An object that is created while it is not running is either refused or, if the webhook is not installed, makes the operator exit. |
| ETCD-020, ETCD-031, ETCD-041 | The cluster Service is backed by the cluster addresses of other Services, which Kubernetes does not route. Nothing reaches a member through it, so the `Etcd` object never becomes ready, and no second member can join. |
| ETCD-025, ETCD-050, ETCD-051 | Members are added and removed all at once. |
| ETCD-026 | A missing Secret is waited for without retrying. |
| ETCD-034 | Members are looked up in all namespaces by the name of the `Etcd` object. |
| ETCD-047, ETCD-048 | The `Etcd` object is not reconciled when its members change, only when something else touches it. |
| ETCD-053 | A member is removed only by a command that runs inside its own virtual machine. |
| ETCD-064 | A change before the first readiness scales the cluster to zero members, and it stays there. |
| ETCD-071 | Deletion waits for every member to remove itself, forever if one cannot. |
| ETCD-080 | An error while creating or updating the members makes the operator exit. |
| ETCD-081 | SSH connections have no timeout, and one that hangs blocks all clusters. |

Whether the remaining requirements hold today is not known. The code has never been tested, and it has not formed a
cluster of more than one member on a current Kubernetes.

## Known limitations

These are true of the operator, are not requirements, and are not changed by this specification. Nobody should rely
on them staying this way.

**Security**

- The operator does not verify the certificates of the members it talks to, and does not verify the SSH host keys of
  the virtual machines.
- The private key of the certificate authority is copied into every virtual machine, and is readable in the Secret
  that holds the cloud-init data of each one.
- Certificates are valid for ten years and are never renewed. All certificates of a cluster have the same serial
  number.
- The cluster Service and a Service per member are of the type `NodePort`, so the etcd and SSH ports of every member
  are reachable on every node of the Kubernetes cluster.
- The tool that sets up etcd, etcdadm, is downloaded into every virtual machine when it starts, without a checksum.
  It is no longer maintained.

**Resilience**

- The data of a member lives on a disk that is deleted with its virtual machine.
- A member whose virtual machine is deleted or fails is not replaced. The virtual machine is created again, but etcd
  is not set up in it.
- The first member is replaced once, right after the cluster becomes ready for the first time.
- When the Secrets of ETCD-010 to ETCD-012 are deleted, they are created again with new keys, which the running
  members do not have.
- Nothing prevents changing `spec.version` to a lower version, which etcd does not support.

**Reporting**

- `status.replicas` repeats `spec.replicas` and says nothing about the members that exist.
- `status.observedGeneration` is never set.
- The Secret `peer-<name>` and the status fields that name it exist but are not used for anything.
- Conditions have no `reason`, and their `lastProbeTime` is the last time they were `True`, not the last time they
  were checked.

## Open questions

None. The decisions that were taken while drafting are listed in the [plan](plan.md#decisions), so that they can be
overruled.
