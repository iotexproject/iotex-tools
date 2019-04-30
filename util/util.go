// Copyright (c) 2019 IoTeX
// This program is free software: you can redistribute it and/or modify it under the terms of the
// GNU General Public License as published by the Free Software Foundation, either version 3 of
// the License, or (at your option) any later version.
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See
// the GNU General Public License for more details.
// You should have received a copy of the GNU General Public License along with this program. If
// not, see <http://www.gnu.org/licenses/>.

package util

import (
	"io/ioutil"

	"github.com/iotexproject/iotex-election/committee"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// NewCommitteeWithConfigFile creates a committee with config file
func NewCommitteeWithConfigFile(filename string) (committee.Committee, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load config file %s", filename)
	}
	var config committee.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}
	return committee.NewCommittee(nil, config)
}
