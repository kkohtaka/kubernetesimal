/*
Copyright 2019 Kazumasa Kohtaka <kkohtaka@gmail.com>.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package matchbox

import (
	"encoding/base64"
	"fmt"

	ignition "github.com/coreos/ignition/config/v2_3/types"
)

//go:generate go run ../../../hack/embedtexts.go templates matchbox

// TODO: Generate certificates
var (
	caCertContent     = ``
	serverCertContent = ``
	serverKeyContent  = ``
)

func newIgnitionConfig() *ignition.Config {
	c := ignition.Config{
		Ignition: ignition.Ignition{Version: "2.3"},
		Systemd:  newIgnitionSystemd("matchbox.service", matchboxService),
		Storage: ignition.Storage{
			Files: []ignition.File{
				newIgnitionFile("/etc/systemd/system/matchbox.service.d/override.conf", matchboxOverrideConf),
				newIgnitionFile("/etc/matchbox/ca.crt", caCertContent),
				newIgnitionFile("/etc/matchbox/server.crt", serverCertContent),
				newIgnitionFile("/etc/matchbox/server.key", serverKeyContent),
			},
		},
	}
	return &c
}

func newIgnitionSystemd(name, content string) ignition.Systemd {
	return ignition.Systemd{
		Units: []ignition.Unit{
			ignition.Unit{
				Name:     name,
				Contents: content,
			},
		},
	}
}

func newIgnitionFile(path, content string) ignition.File {
	return ignition.File{
		Node: ignition.Node{
			Filesystem: "root",
			Path:       path,
		},
		FileEmbedded1: ignition.FileEmbedded1{
			Contents: ignition.FileContents{
				Source: encodeDataURL(content),
			},
		},
	}
}

func encodeDataURL(content string) string {
	const mime = "text/plain"
	base64 := base64.StdEncoding.EncodeToString([]byte(content))
	return fmt.Sprintf("data:%s;charset=utf-8;base64,%s", mime, base64)
}
