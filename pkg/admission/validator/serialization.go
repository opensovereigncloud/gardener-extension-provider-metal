// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"github.com/gardener/gardener/extensions/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/ironcore-dev/gardener-extension-provider-ironcore-metal/pkg/apis/metal"
)

func decodeCloudProfileConfig(decoder runtime.Decoder, config *runtime.RawExtension) (*metal.CloudProfileConfig, error) {
	cloudProfileConfig := &metal.CloudProfileConfig{}
	if err := util.Decode(decoder, config.Raw, cloudProfileConfig); err != nil {
		return nil, err
	}
	return cloudProfileConfig, nil
}
