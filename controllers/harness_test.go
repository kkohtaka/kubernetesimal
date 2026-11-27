/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controllers

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/etcdclient"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

const (
	// timeout is how long a test waits for the controllers. The longest wait in the controllers that these tests
	// pass through is the five seconds between two attempts to reach a machine or to probe a member.
	timeout  = time.Minute
	interval = 250 * time.Millisecond

	// bootImage is the name of the PersistentVolumeClaim that the Etcd objects of the tests name as their image.
	bootImage = "boot-image"

	startScript = "sudo /opt/bin/start-cluster.sh"
	leaveScript = "sudo /opt/bin/leave-cluster.sh"
)

// newNamespace creates a namespace for one test. envtest runs no namespace controller, so namespaces are never
// removed, and every test gets its own.
func newNamespace(ctx context.Context) string {
	namespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
	ExpectWithOffset(1, k8sClient.Create(ctx, &namespace)).To(Succeed())
	return namespace.Name
}

// newEtcd creates an Etcd object of one member. The suite runs no webhook, so nothing is defaulted.
//
// The object is named after its namespace, so that no two tests use the same name. The operator looks for the
// members of a cluster by the name of the Etcd object in all namespaces, so two objects with one name would count
// each other's members. ETCD-034 requires otherwise, and its test is where a shared name belongs.
//
// Each change is applied to the object before it is created.
func newEtcd(
	ctx context.Context,
	namespace string,
	changes ...func(*kubernetesimalv1alpha1.Etcd),
) *kubernetesimalv1alpha1.Etcd {
	version, replicas := "3.5.1", int32(1)
	e := kubernetesimalv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "etcd-" + namespace},
		Spec: kubernetesimalv1alpha1.EtcdSpec{
			Version:                       &version,
			Replicas:                      &replicas,
			ImagePersistentVolumeClaimRef: corev1.LocalObjectReference{Name: bootImage},
		},
	}
	for _, change := range changes {
		change(&e)
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, &e)).To(Succeed())
	return &e
}

// getNode returns the only EtcdNode of a namespace, once it names its Service and its virtual machine.
func getNode(ctx context.Context, namespace string) (*kubernetesimalv1alpha1.EtcdNode, error) {
	var nodes kubernetesimalv1alpha1.EtcdNodeList
	if err := k8sClient.List(ctx, &nodes, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	if len(nodes.Items) != 1 {
		return nil, fmt.Errorf("the namespace has %d EtcdNode objects", len(nodes.Items))
	}
	node := &nodes.Items[0]
	if node.Status.PeerServiceRef == nil || node.Status.VirtualMachineInstanceRef == nil {
		return nil, errors.New("the EtcdNode does not name its Service and its virtual machine yet")
	}
	return node, nil
}

// waitForNode waits for the only EtcdNode of a namespace and returns it with the address of its Service, which is
// where the operator reaches its machine.
func waitForNode(ctx context.Context, namespace string) (*kubernetesimalv1alpha1.EtcdNode, string) {
	var node *kubernetesimalv1alpha1.EtcdNode
	EventuallyWithOffset(1, func() (err error) {
		node, err = getNode(ctx, namespace)
		return err
	}, timeout, interval).Should(Succeed())

	var service corev1.Service
	ExpectWithOffset(1, k8sClient.Get(
		ctx,
		types.NamespacedName{Namespace: namespace, Name: node.Status.PeerServiceRef.Name},
		&service,
	)).To(Succeed())
	ExpectWithOffset(1, service.Spec.ClusterIP).NotTo(BeEmpty())
	return node, service.Spec.ClusterIP
}

// nodeStatus reads the status of one EtcdNode. It asks for the node by name, because the operator adds a second
// EtcdNode to a cluster once the cluster has been ready.
func nodeStatus(
	ctx context.Context,
	node *kubernetesimalv1alpha1.EtcdNode,
) func() (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	return func() (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
		var current kubernetesimalv1alpha1.EtcdNode
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(node), &current); err != nil {
			return nil, err
		}
		return &current.Status, nil
	}
}

func etcdStatus(ctx context.Context, e *kubernetesimalv1alpha1.Etcd) func() (*kubernetesimalv1alpha1.EtcdStatus, error) {
	return func() (*kubernetesimalv1alpha1.EtcdStatus, error) {
		var current kubernetesimalv1alpha1.Etcd
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &current); err != nil {
			return nil, err
		}
		return &current.Status, nil
	}
}

// derOf returns the first certificate of a PEM file in the form that a TLS configuration holds it.
func derOf(certificate []byte) []byte {
	block, _ := pem.Decode(certificate)
	ExpectWithOffset(1, block).NotTo(BeNil())
	return block.Bytes
}

// commands returns the commands of some calls, in order.
//
// The tests do not expect a script to run exactly once. The operator sometimes runs the start script a second time
// on a machine, when it reconciles an EtcdNode again before it sees that the first run was recorded in the status.
// Whether that is acceptable is a question for the tests of the requirements, not for the tests of the harness.
func commands(calls []commandCall) []string {
	var commands []string
	for _, call := range calls {
		commands = append(commands, call.Command)
	}
	return commands
}

var _ = Describe("The operator in envtest", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resetFakes()
		namespace = newNamespace(ctx)
	})

	AfterEach(func() {
		resetFakes()
	})

	Describe("reaching a virtual machine", func() {
		It("runs the start script on the machine of the first member, with the SSH key of the cluster", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)

			Eventually(func() []commandCall { return machines.CallsTo(address) }, timeout, interval).
				ShouldNot(BeEmpty())
			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeTrue()))

			var keyPair corev1.Secret
			Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{Namespace: namespace, Name: "ssh-keypair-" + e.Name},
				&keyPair,
			)).To(Succeed())

			// The operator may run the script more than once: see the note on commands.
			calls := machines.CallsTo(address)
			Expect(commands(calls)).To(HaveEach(startScript))
			for _, call := range calls {
				Expect(call.Port).To(Equal(22))
				Expect(call.PrivateKey).To(Equal(keyPair.Data[corev1.SSHAuthPrivateKey]))
			}
		})

		It("runs nothing on a machine before KubeVirt reports it as running", func() {
			kubeVirt.Hold(namespace)
			newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)

			Consistently(func() []commandCall { return machines.CallsTo(address) }, 3*time.Second, interval).
				Should(BeEmpty())
			Expect(nodeStatus(ctx, node)()).
				To(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeFalse()))

			Expect(kubeVirt.Release(ctx, namespace)).To(Succeed())

			Eventually(func() []string { return commands(machines.CallsTo(address)) }, timeout, interval).
				Should(SatisfyAll(Not(BeEmpty()), HaveEach(startScript)))
		})

		It("tries again when the machine cannot be reached, and says so in the status", func() {
			machines.Answer(func(commandCall) error {
				return &ssh.ConnectError{Err: errors.New("could not dial: connection refused")}
			})
			newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)

			Eventually(func() int { return len(machines.CallsTo(address)) }, timeout, interval).
				Should(BeNumerically(">=", 2))

			status, err := nodeStatus(ctx, node)()
			Expect(err).NotTo(HaveOccurred())
			Expect(status.IsProvisioned()).To(BeFalse())
			Expect(status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", kubernetesimalv1alpha1.EtcdNodeConditionTypeProvisioned),
				HaveField("Status", corev1.ConditionFalse),
				HaveField("Message", "waiting for an SSH port of an etcd member prepared: "+
					"could not dial: connection refused"),
			)))

			machines.Answer(nil)

			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeTrue()))
		})

		It("reports a command that ran and failed in the status", func() {
			machines.Answer(func(commandCall) error {
				return errors.New("unable to complete a command: Process exited with status 1")
			})
			newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)

			Eventually(func() []commandCall { return machines.CallsTo(address) }, timeout, interval).
				ShouldNot(BeEmpty())
			Eventually(nodeStatus(ctx, node), timeout, interval).Should(HaveField("Conditions", ContainElement(SatisfyAll(
				HaveField("Type", kubernetesimalv1alpha1.EtcdNodeConditionTypeProvisioned),
				HaveField("Status", corev1.ConditionFalse),
				HaveField("Message", "unable to complete a command: Process exited with status 1"),
			))))
		})

		It("runs the leave script on the machine of a member when the Etcd object is deleted", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)
			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeTrue()))

			Expect(k8sClient.Delete(ctx, e)).To(Succeed())

			Eventually(func() []string { return commands(machines.CallsTo(address)) }, timeout, interval).
				Should(ContainElement(leaveScript))
			ran := commands(machines.CallsTo(address))
			Expect(ran[len(ran)-1]).To(Equal(leaveScript))
			Expect(ran[:len(ran)-1]).To(SatisfyAll(Not(BeEmpty()), HaveEach(startScript)))
			Eventually(func() bool {
				var vmi kubevirtv1.VirtualMachineInstance
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{Namespace: namespace, Name: node.Status.VirtualMachineInstanceRef.Name},
					&vmi,
				)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Describe("reaching etcd", func() {
		// clusterAddress returns the address of the cluster Service, which is where the operator reaches the cluster.
		clusterAddress := func(e *kubernetesimalv1alpha1.Etcd) string {
			var service corev1.Service
			Eventually(func() (string, error) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &service)
				return service.Spec.ClusterIP, err
			}, timeout, interval).ShouldNot(BeEmpty())
			return fmt.Sprintf("%s:2379", service.Spec.ClusterIP)
		}

		It("reports a member and its cluster as not ready when etcd does not answer", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)
			memberAddress := fmt.Sprintf("%s:2379", address)

			Eventually(func() []etcdCall {
				return etcdCluster.CallsTo(etcdMethodHealth, memberAddress)
			}, timeout, interval).ShouldNot(BeEmpty())
			Eventually(nodeStatus(ctx, node), timeout, interval).Should(HaveField("Conditions", ContainElement(SatisfyAll(
				HaveField("Type", kubernetesimalv1alpha1.EtcdNodeConditionTypeReady),
				HaveField("Status", corev1.ConditionFalse),
			))))

			Eventually(func() []etcdCall {
				return etcdCluster.CallsTo(etcdMethodHealth, clusterAddress(e))
			}, timeout, interval).ShouldNot(BeEmpty())
			Eventually(etcdStatus(ctx, e), timeout, interval).Should(HaveField("Conditions", ContainElements(
				SatisfyAll(
					HaveField("Type", kubernetesimalv1alpha1.EtcdConditionTypeReady),
					HaveField("Status", corev1.ConditionFalse),
				),
				SatisfyAll(
					HaveField("Type", kubernetesimalv1alpha1.EtcdConditionTypeMembersHealthy),
					HaveField("Status", corev1.ConditionFalse),
					HaveField("Message", "unable to list etcd members: the fake cluster does not answer"),
				),
			)))
		})

		It("reports a member and its cluster as ready when etcd answers, and asks with the client certificate", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)
			memberAddress := fmt.Sprintf("%s:2379", address)
			memberURL := fmt.Sprintf("https://%s", memberAddress)
			cluster := clusterAddress(e)

			// The member answers first, and the cluster only when the member is ready. Once the cluster has been
			// ready, the operator adds a second EtcdNode, and the member list below would no longer match.
			etcdCluster.AnswerHealth(func(address string) (bool, error) {
				return address == memberAddress, nil
			})
			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsReady, BeTrue()))

			etcdCluster.AnswerMemberList(func(address string) ([]etcdclient.Member, error) {
				if address != cluster {
					return nil, errors.New("unable to list etcd members: the fake cluster does not answer")
				}
				return []etcdclient.Member{{Name: node.Name, ClientURLs: []string{memberURL}}}, nil
			})
			etcdCluster.AnswerMemberStatus(func(clientURL string) error {
				if clientURL != memberURL {
					return errors.New("the fake member does not answer")
				}
				return nil
			})
			etcdCluster.AnswerHealth(func(address string) (bool, error) {
				return address == memberAddress || address == cluster, nil
			})

			Eventually(etcdStatus(ctx, e), timeout, interval).Should(HaveField("Conditions", ContainElements(
				SatisfyAll(
					HaveField("Type", kubernetesimalv1alpha1.EtcdConditionTypeReady),
					HaveField("Status", corev1.ConditionTrue),
				),
				SatisfyAll(
					HaveField("Type", kubernetesimalv1alpha1.EtcdConditionTypeMembersHealthy),
					HaveField("Status", corev1.ConditionTrue),
				),
			)))

			var clientCertificate corev1.Secret
			Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{Namespace: namespace, Name: "api-client-" + e.Name},
				&clientCertificate,
			)).To(Succeed())

			for _, calls := range [][]etcdCall{
				etcdCluster.CallsTo(etcdMethodHealth, memberAddress),
				etcdCluster.CallsTo(etcdMethodHealth, cluster),
				etcdCluster.CallsTo(etcdMethodMemberList, cluster),
				etcdCluster.CallsTo(etcdMethodMemberStatus, memberURL),
			} {
				Expect(calls).NotTo(BeEmpty())
				last := calls[len(calls)-1]
				Expect(last.TLSConfig).NotTo(BeNil())
				Expect(last.TLSConfig.Certificates).To(HaveLen(1))
				Expect(last.TLSConfig.Certificates[0].Certificate[0]).
					To(Equal(derOf(clientCertificate.Data[corev1.TLSCertKey])))
			}
		})
	})
})
