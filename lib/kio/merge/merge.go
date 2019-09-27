// Copyright 2019 Google LLC
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

// Package merge contains libraries for merging Resources and Patches
package merge

import (
	"lib.kpt.dev/yaml"
	"lib.kpt.dev/yaml/walk/merge"
)

// Filter merges Resources with the Group/Version/Kind/Namespace/Name together using
// a 2-way merge strategy.
//
// - Fields set to null in the source will be cleared from the destination
// - Fields with matching keys will be merged recursively
// - Lists with an associative key (e.g. name) will have their elements merged using the key
// - List without an associative key will have the dest list replaced by the source list
type Filter struct{}

type mergeKey struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
}

// Filter implements kio.Filter by merge Resources with the same G/V/K/NS/N
func (c Filter) Filter(input []*yaml.RNode) ([]*yaml.RNode, error) {
	// index the Resources by G/V/K/NS/N
	index := map[mergeKey][]*yaml.RNode{}
	for i := range input {
		meta, err := input[i].GetMeta()
		if err != nil {
			return nil, err
		}
		key := mergeKey{
			apiVersion: meta.ApiVersion,
			kind:       meta.Kind,
			namespace:  meta.Namespace,
			name:       meta.Name,
		}
		index[key] = append(index[key], input[i])
	}

	// merge each of the G/V/K/NS/N lists
	var output []*yaml.RNode
	var err error
	for k := range index {
		var merged *yaml.RNode
		resources := index[k]
		for i := range resources {
			patch := resources[i]
			if merged == nil {
				// first resources, don't merge it
				merged = resources[i]
			} else {
				merged, err = merge.Merge(patch, merged)
				if err != nil {
					return nil, err
				}
			}
		}
		output = append(output, merged)
	}
	return output, nil
}
