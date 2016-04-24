// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package drive

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/odeke-em/extractor"
	"github.com/odeke-em/meddler"
	"github.com/skratchdot/open-golang/open"
)

var envKeyAlias = &extractor.EnvKey{
	PubKeyAlias:  "DRIVE_SERVER_PUB_KEY",
	PrivKeyAlias: "DRIVE_SERVER_PRIV_KEY",
}

const (
	DefaultQRShareServer = "https://qr-server.herokuapp.com/drive"
)

var envKeySet = extractor.KeySetFromEnv(envKeyAlias)

func (g *Commands) QR(byId bool) error {

	kvChan := g.urler(byId, g.opts.Sources)

	address := DefaultQRShareServer
	if g.opts.Meta != nil {
		meta := *(g.opts.Meta)
		if retrAddress, ok := meta[AddressKey]; ok && len(retrAddress) >= 1 {
			address = retrAddress[0]
		}
	}

	address = strings.TrimRight(address, "/")
	if envKeySet.PublicKey == "" {
		envKeySet.PublicKey = "5160897b3586461e83e7279c10352ac4"
	}
	if envKeySet.PrivateKey == "" {
		envKeySet.PrivateKey = "5a3451dadab74f75b16f754c0a931949"
	}

	for kv := range kvChan {
		switch kv.value.(type) {
		case error:
			g.log.LogErrf("%s: %s\n", kv.key, kv.value)
			continue
		}

		fileURI, ok := kv.value.(string)
		if !ok {
			continue
		}

		curTime := time.Now()
		pl := meddler.Payload{
			URI:         fileURI,
			RequestTime: curTime.Unix(),
			Payload:     fmt.Sprintf("%v%v", rand.Float64(), curTime),
			PublicKey:   envKeySet.PublicKey,
			ExpiryTime:  curTime.Add(time.Hour).Unix(),
		}

		plainTextToSign := pl.RawTextForSigning()

		pl.Signature = fmt.Sprintf("%s", envKeySet.Sign([]byte(plainTextToSign)))

		uv := pl.ToUrlValues()
		encodedValues := uv.Encode()

		fullUrl := fmt.Sprintf("%s/qr?%s", address, encodedValues)
		if g.opts.Verbose {
			g.log.Logf("%q => %q\n", kv.key, fullUrl)
		}

		if err := open.Start(fullUrl); err != nil {
			g.log.Logf("qr: err %v %q\n", err, fullUrl)
		}
	}

	return nil
}
