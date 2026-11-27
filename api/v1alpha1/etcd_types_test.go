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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The API server fills in the version and the number of replicas of every Etcd object, see the tests of ETCD-001
// and ETCD-002. An object that the operator builds or reads in another way may still lack them.

func TestEtcdSpecVersionOrDefault(t *testing.T) {
	version := "3.5.6"
	assert.Equal(t, "3.5.6", (&EtcdSpec{Version: &version}).VersionOrDefault())
	assert.Equal(t, "3.5.1", (&EtcdSpec{}).VersionOrDefault())
}

func TestEtcdSpecReplicasOrDefault(t *testing.T) {
	replicas := int32(3)
	assert.Equal(t, int32(3), (&EtcdSpec{Replicas: &replicas}).ReplicasOrDefault())
	assert.Equal(t, int32(1), (&EtcdSpec{}).ReplicasOrDefault())
}
