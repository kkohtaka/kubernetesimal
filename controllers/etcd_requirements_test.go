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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/etcdclient"
)

// These tests claim requirements of docs/specs/etcd/spec.md that the operator is expected to meet already. They are
// step 2 of the plan of that specification.

// getSecret waits for a Secret and returns it.
func getSecret(ctx context.Context, namespace, name string) *corev1.Secret {
	var secret corev1.Secret
	EventuallyWithOffset(1, func() error {
		return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret)
	}, timeout, interval).Should(Succeed())
	return &secret
}

// parseCertificate reads the only certificate of a PEM file.
func parseCertificate(data []byte) *x509.Certificate {
	block, rest := pem.Decode(data)
	ExpectWithOffset(1, block).NotTo(BeNil(), "the data is not PEM")
	ExpectWithOffset(1, block.Type).To(Equal("CERTIFICATE"))
	ExpectWithOffset(1, rest).To(BeEmpty(), "the data holds more than one PEM block")
	certificate, err := x509.ParseCertificate(block.Bytes)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return certificate
}

// getVirtualMachine returns the VirtualMachineInstance of an EtcdNode.
func getVirtualMachine(
	ctx context.Context,
	node *kubernetesimalv1alpha1.EtcdNode,
) *kubevirtv1.VirtualMachineInstance {
	var vmi kubevirtv1.VirtualMachineInstance
	ExpectWithOffset(1, k8sClient.Get(
		ctx,
		types.NamespacedName{Namespace: node.Namespace, Name: node.Status.VirtualMachineInstanceRef.Name},
		&vmi,
	)).To(Succeed())
	return &vmi
}

// getCloudConfig returns the cloud-init user data that a virtual machine is given, parsed as the cloud-config that it
// has to be.
func getCloudConfig(ctx context.Context, vmi *kubevirtv1.VirtualMachineInstance) map[string]any {
	var secretName string
	for _, volume := range vmi.Spec.Volumes {
		if volume.CloudInitNoCloud != nil {
			ExpectWithOffset(1, volume.CloudInitNoCloud.UserDataSecretRef).NotTo(BeNil())
			secretName = volume.CloudInitNoCloud.UserDataSecretRef.Name
		}
	}
	ExpectWithOffset(1, secretName).NotTo(BeEmpty(), "the virtual machine is given no cloud-init data")

	// KubeVirt reads the user data from the key "userdata" of the Secret.
	userData := getSecret(ctx, vmi.Namespace, secretName).Data["userdata"]
	ExpectWithOffset(1, string(userData)).To(MatchRegexp(`^\s*#cloud-config\n`))

	var config map[string]any
	ExpectWithOffset(1, yaml.Unmarshal(userData, &config)).To(Succeed())
	return config
}

// answerForMember makes the fake etcd answer for one member, and for nothing else.
func answerForMember(memberAddress string) {
	etcdCluster.AnswerHealth(func(address string) (bool, error) {
		return address == memberAddress, nil
	})
}

// answerForCluster makes the fake etcd answer for one member and for the cluster that it forms alone: the health
// check through the cluster Service succeeds, and the member list has that one member, which answers.
func answerForCluster(node *kubernetesimalv1alpha1.EtcdNode, memberAddress, clusterAddress string) {
	memberURL := fmt.Sprintf("https://%s", memberAddress)
	etcdCluster.AnswerMemberList(func(address string) ([]etcdclient.Member, error) {
		if address != clusterAddress {
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
		return address == memberAddress || address == clusterAddress, nil
	})
}

// getClusterAddress waits for the cluster Service and returns where the operator reaches the cluster through it.
func getClusterAddress(ctx context.Context, e *kubernetesimalv1alpha1.Etcd) string {
	var service corev1.Service
	EventuallyWithOffset(1, func() (string, error) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &service)
		return service.Spec.ClusterIP, err
	}, timeout, interval).ShouldNot(BeEmpty())
	return fmt.Sprintf("%s:2379", service.Spec.ClusterIP)
}

// reconciledStatus reads the status of an Etcd object after asking the operator to reconcile the object.
//
// The operator does not reconcile an Etcd object when its members change, only when the object itself changes. That
// is a known difference from ETCD-047 and ETCD-048, which say how soon the status follows the members. The
// requirements that these tests claim say what the status is, not when, so the tests ask for the reconciliation
// themselves, by changing an annotation.
func reconciledStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
) func() (*kubernetesimalv1alpha1.EtcdStatus, error) {
	return func() (*kubernetesimalv1alpha1.EtcdStatus, error) {
		var current kubernetesimalv1alpha1.Etcd
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &current); err != nil {
			return nil, err
		}
		patch := client.MergeFrom(current.DeepCopy())
		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations["test.kubernetesimal.kkohtaka.org/reconcile"] = time.Now().Format(time.RFC3339Nano)
		if err := k8sClient.Patch(ctx, &current, patch); err != nil {
			return nil, err
		}
		return &current.Status, nil
	}
}

// haveReady matches a status whose condition Ready has the given value.
func haveReady(status corev1.ConditionStatus) OmegaMatcher {
	return HaveField("Conditions", ContainElement(SatisfyAll(
		HaveField("Type", kubernetesimalv1alpha1.EtcdConditionTypeReady),
		HaveField("Status", status),
	)))
}

var _ = Describe("An etcd cluster", func() {
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

	Describe("credentials", func() {
		It("[ETCD-010] has a Secret with a self-signed certificate authority and its private key", func() {
			e := newEtcd(ctx, namespace)

			secret := getSecret(ctx, namespace, "ca-"+e.Name)
			Expect(secret.Type).To(Equal(corev1.SecretTypeTLS))

			authority := parseCertificate(secret.Data[corev1.TLSCertKey])
			Expect(authority.IsCA).To(BeTrue())
			Expect(authority.BasicConstraintsValid).To(BeTrue())
			Expect(authority.Subject.String()).To(Equal(authority.Issuer.String()))
			Expect(authority.CheckSignatureFrom(authority)).To(Succeed(), "the certificate is not signed by itself")

			_, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
			Expect(err).NotTo(HaveOccurred(), "the private key does not match the certificate")
		})

		It("[ETCD-011] has a Secret with a client certificate of that authority and its private key", func() {
			e := newEtcd(ctx, namespace)

			secret := getSecret(ctx, namespace, "api-client-"+e.Name)
			Expect(secret.Type).To(Equal(corev1.SecretTypeTLS))

			authorities := x509.NewCertPool()
			authorities.AddCert(parseCertificate(getSecret(ctx, namespace, "ca-"+e.Name).Data[corev1.TLSCertKey]))

			_, err := parseCertificate(secret.Data[corev1.TLSCertKey]).Verify(x509.VerifyOptions{
				Roots:     authorities,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			})
			Expect(err).NotTo(HaveOccurred(), "the certificate is not a client certificate of the authority")

			_, err = tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
			Expect(err).NotTo(HaveOccurred(), "the private key does not match the certificate")
		})

		It("[ETCD-012] has a Secret with an SSH private key and the matching public key", func() {
			e := newEtcd(ctx, namespace)

			secret := getSecret(ctx, namespace, "ssh-keypair-"+e.Name)
			Expect(secret.Type).To(Equal(corev1.SecretTypeSSHAuth))

			block, _ := pem.Decode(secret.Data["ssh-privatekey"])
			Expect(block).NotTo(BeNil(), "the private key is not in PEM format")
			signer, err := ssh.ParsePrivateKey(secret.Data["ssh-privatekey"])
			Expect(err).NotTo(HaveOccurred())

			publicKey, _, _, rest, err := ssh.ParseAuthorizedKey(secret.Data["ssh-publickey"])
			Expect(err).NotTo(HaveOccurred(), "the public key is not a line of an authorized_keys file")
			Expect(rest).To(BeEmpty(), "the public key is more than one line")
			Expect(publicKey.Marshal()).To(Equal(signer.PublicKey().Marshal()), "the keys do not match")
		})

		It("[ETCD-013] names the Secrets and their keys in its status", func() {
			e := newEtcd(ctx, namespace)

			selector := func(name, key string) *corev1.SecretKeySelector {
				return &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: key}
			}
			Eventually(etcdStatus(ctx, e), timeout, interval).Should(SatisfyAll(
				HaveField("CACertificateRef", Equal(selector("ca-"+e.Name, "tls.crt"))),
				HaveField("CAPrivateKeyRef", Equal(selector("ca-"+e.Name, "tls.key"))),
				HaveField("ClientCertificateRef", Equal(selector("api-client-"+e.Name, "tls.crt"))),
				HaveField("ClientPrivateKeyRef", Equal(selector("api-client-"+e.Name, "tls.key"))),
				HaveField("SSHPrivateKeyRef", Equal(selector("ssh-keypair-"+e.Name, "ssh-privatekey"))),
				HaveField("SSHPublicKeyRef", Equal(selector("ssh-keypair-"+e.Name, "ssh-publickey"))),
			))

			// The names in the status are names of Secrets that exist and have those keys.
			for name, keys := range map[string][]string{
				"ca-" + e.Name:          {"tls.crt", "tls.key"},
				"api-client-" + e.Name:  {"tls.crt", "tls.key"},
				"ssh-keypair-" + e.Name: {"ssh-privatekey", "ssh-publickey"},
			} {
				secret := getSecret(ctx, namespace, name)
				for _, key := range keys {
					Expect(secret.Data).To(HaveKey(key))
				}
			}
		})

		It("[ETCD-014] keeps the contents of the Secrets when the object is reconciled and changed", func() {
			e := newEtcd(ctx, namespace)
			waitForNode(ctx, namespace)

			names := []string{"ca-" + e.Name, "api-client-" + e.Name, "ssh-keypair-" + e.Name}
			contents := func() map[string]map[string][]byte {
				contents := map[string]map[string][]byte{}
				for _, name := range names {
					contents[name] = getSecret(ctx, namespace, name).Data
				}
				return contents
			}
			before := contents()
			for _, name := range names {
				Expect(before[name]).NotTo(BeEmpty())
			}

			By("reconciling the object without changing its specification")
			for i := 0; i < 3; i++ {
				_, err := reconciledStatus(ctx, e)()
				Expect(err).NotTo(HaveOccurred())
			}

			By("changing the number of replicas")
			var current kubernetesimalv1alpha1.Etcd
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &current)).To(Succeed())
			patch := client.MergeFrom(current.DeepCopy())
			replicas := int32(3)
			current.Spec.Replicas = &replicas
			Expect(k8sClient.Patch(ctx, &current, patch)).To(Succeed())
			// The status repeats the number, which shows that the operator has reconciled the change.
			Eventually(etcdStatus(ctx, e), timeout, interval).Should(HaveField("Replicas", int32(3)))
			Expect(contents()).To(Equal(before))

			By("changing the version")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &current)).To(Succeed())
			patch = client.MergeFrom(current.DeepCopy())
			version := "3.5.6"
			current.Spec.Version = &version
			Expect(k8sClient.Patch(ctx, &current, patch)).To(Succeed())

			Consistently(contents, 5*time.Second, interval).Should(Equal(before))
		})
	})

	Describe("virtual machines", func() {
		It("[ETCD-022] boots a member from a copy of the image, which it does not write to", func() {
			newEtcd(ctx, namespace)
			node, _ := waitForNode(ctx, namespace)
			vmi := getVirtualMachine(ctx, node)

			// An ephemeral volume is KubeVirt's copy-on-write layer over a PersistentVolumeClaim: the virtual machine
			// boots from the image and writes to a local file, so that the claim is not changed and can be shared.
			var imageVolumes []string
			for _, volume := range vmi.Spec.Volumes {
				Expect(volume.PersistentVolumeClaim).To(BeNil(), "volume %q attaches a claim directly", volume.Name)
				Expect(volume.DataVolume).To(BeNil(), "volume %q attaches a data volume directly", volume.Name)
				if volume.Ephemeral == nil {
					continue
				}
				Expect(volume.Ephemeral.PersistentVolumeClaim).NotTo(BeNil())
				Expect(volume.Ephemeral.PersistentVolumeClaim.ClaimName).To(Equal(bootImage))
				Expect(volume.Ephemeral.PersistentVolumeClaim.ReadOnly).To(BeTrue())
				imageVolumes = append(imageVolumes, volume.Name)
			}
			Expect(imageVolumes).To(HaveLen(1))

			// KubeVirt boots from the first disk, unless a disk is given a boot order.
			disks := vmi.Spec.Domain.Devices.Disks
			Expect(disks).NotTo(BeEmpty())
			Expect(disks[0].Name).To(Equal(imageVolumes[0]))
			for _, disk := range disks {
				Expect(disk.BootOrder).To(BeNil())
			}
		})

		It("[ETCD-023] [ETCD-024] sets the login password from the Secret, and keeps SSH from accepting it", func() {
			password := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "login"},
				StringData: map[string]string{"password": "correct horse: battery #staple"},
			}
			Expect(k8sClient.Create(ctx, &password)).To(Succeed())

			newEtcd(ctx, namespace, func(e *kubernetesimalv1alpha1.Etcd) {
				e.Spec.LoginPasswordSecretKeySelector = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "login"},
					Key:                  "password",
				}
			})
			node, _ := waitForNode(ctx, namespace)
			config := getCloudConfig(ctx, getVirtualMachine(ctx, node))

			// "password" sets the password of the default user of the image, which is "fedora" for the images that
			// the specification asks for. It would expire at the first login without the setting below.
			Expect(config).To(HaveKeyWithValue("password", "correct horse: battery #staple"))
			Expect(config).To(HaveKeyWithValue("chpasswd", HaveKeyWithValue("expire", false)))

			Expect(config).To(HaveKeyWithValue("ssh_pwauth", false))
		})

		It("[ETCD-023] [ETCD-024] sets no password without a Secret, and keeps SSH from accepting one", func() {
			newEtcd(ctx, namespace)
			node, _ := waitForNode(ctx, namespace)
			config := getCloudConfig(ctx, getVirtualMachine(ctx, node))

			Expect(config).NotTo(HaveKey("password"))
			// The other ways in which a cloud-config can set a password.
			Expect(config).NotTo(HaveKey("users"))
			Expect(config).To(HaveKeyWithValue("chpasswd", Not(Or(HaveKey("list"), HaveKey("users")))))

			Expect(config).To(HaveKeyWithValue("ssh_pwauth", false))
		})
	})

	Describe("the cluster Service", func() {
		It("[ETCD-030] has the name of the object and a port named etcd on port 2379", func() {
			e := newEtcd(ctx, namespace)

			var service corev1.Service
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &service)
			}, timeout, interval).Should(Succeed())
			Expect(service.Spec.Ports).To(ContainElement(SatisfyAll(
				HaveField("Name", "etcd"),
				HaveField("Port", int32(2379)),
			)))
		})

		It("[ETCD-033] is named in the status", func() {
			e := newEtcd(ctx, namespace)

			Eventually(etcdStatus(ctx, e), timeout, interval).
				Should(HaveField("ServiceRef", Equal(&corev1.LocalObjectReference{Name: e.Name})))

			var service corev1.Service
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(e), &service)).To(Succeed())
		})
	})

	Describe("status", func() {
		It("[ETCD-040] counts the members that are ready", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)

			By("having a member that is not ready")
			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeTrue()))
			Consistently(reconciledStatus(ctx, e), 3*time.Second, time.Second).Should(HaveField("ReadyReplicas", int32(0)))

			By("having a member that is ready")
			answerForMember(fmt.Sprintf("%s:2379", address))
			Eventually(reconciledStatus(ctx, e), timeout, time.Second).Should(HaveField("ReadyReplicas", int32(1)))
			Consistently(reconciledStatus(ctx, e), 3*time.Second, time.Second).Should(HaveField("ReadyReplicas", int32(1)))
		})

		It("[ETCD-043] reports the phase Deleting once its deletion is requested", func() {
			e := newEtcd(ctx, namespace)
			waitForNode(ctx, namespace)

			Expect(k8sClient.Delete(ctx, e)).To(Succeed())

			Eventually(etcdStatus(ctx, e), timeout, interval).
				Should(HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseDeleting))
		})

		It("[ETCD-044] reports the phase Running when it is ready and all members are", func() {
			e := newEtcd(ctx, namespace)
			node, address := waitForNode(ctx, namespace)
			memberAddress := fmt.Sprintf("%s:2379", address)

			answerForMember(memberAddress)
			Eventually(reconciledStatus(ctx, e), timeout, time.Second).Should(HaveField("ReadyReplicas", int32(1)))
			answerForCluster(node, memberAddress, getClusterAddress(ctx, e))

			Eventually(etcdStatus(ctx, e), timeout, interval).Should(SatisfyAll(
				haveReady(corev1.ConditionTrue),
				HaveField("ReadyReplicas", int32(1)),
				HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseRunning),
			))
		})

		It("[ETCD-045] reports the phase Error when all members are ready and it is not", func() {
			e := newEtcd(ctx, namespace)
			_, address := waitForNode(ctx, namespace)

			answerForMember(fmt.Sprintf("%s:2379", address))

			Eventually(reconciledStatus(ctx, e), timeout, time.Second).Should(SatisfyAll(
				haveReady(corev1.ConditionFalse),
				HaveField("ReadyReplicas", int32(1)),
				HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseError),
			))
		})

		It("[ETCD-045] [ETCD-046] reports Creating while members are missing, and Error once it was ready and is not",
			func() {
				// Two replicas, of which the operator starts one before the cluster has been ready. The fake etcd
				// answers for that one only, so the number of ready members stays below two.
				e := newEtcd(ctx, namespace, func(e *kubernetesimalv1alpha1.Etcd) {
					replicas := int32(2)
					e.Spec.Replicas = &replicas
				})
				node, address := waitForNode(ctx, namespace)
				memberAddress := fmt.Sprintf("%s:2379", address)
				clusterAddress := getClusterAddress(ctx, e)

				By("being ready with a member missing")
				answerForMember(memberAddress)
				Eventually(nodeStatus(ctx, node), timeout, interval).
					Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsReady, BeTrue()))
				// The health check of the cluster succeeds, and its member list does not answer. The operator
				// checks a cluster again after five seconds while its members are not healthy, and after three
				// minutes otherwise, which is longer than this test waits for the next step.
				etcdCluster.AnswerHealth(func(address string) (bool, error) {
					return address == memberAddress || address == clusterAddress, nil
				})

				Eventually(reconciledStatus(ctx, e), timeout, time.Second).Should(SatisfyAll(
					haveReady(corev1.ConditionTrue),
					HaveField("ReadyReplicas", BeNumerically("<", 2)),
					HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseCreating),
				))

				By("not being ready any more")
				answerForMember(memberAddress)

				Eventually(reconciledStatus(ctx, e), timeout, time.Second).Should(SatisfyAll(
					haveReady(corev1.ConditionFalse),
					HaveField("ReadyReplicas", BeNumerically("<", 2)),
					HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseError),
				))
			})

		It("[ETCD-046] reports the phase Creating while it has not been ready and members are missing", func() {
			e := newEtcd(ctx, namespace)
			node, _ := waitForNode(ctx, namespace)
			Eventually(nodeStatus(ctx, node), timeout, interval).
				Should(WithTransform((*kubernetesimalv1alpha1.EtcdNodeStatus).IsProvisioned, BeTrue()))

			Consistently(reconciledStatus(ctx, e), 3*time.Second, time.Second).Should(SatisfyAll(
				Not(haveReady(corev1.ConditionTrue)),
				HaveField("ReadyReplicas", int32(0)),
				HaveField("Phase", kubernetesimalv1alpha1.EtcdPhaseCreating),
			))
		})
	})
})
