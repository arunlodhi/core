// Copyright (c) 2016 Pani Networks
// All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package exec

import (
	"log"
	"os/exec"
	"strings"
)

// Executable is a facade to exec.Command().Output()
type Executable interface {
	Exec(cmd string, args []string) ([]byte, error)
}

// DefaultExecutor is a default implementation of Executable that passes
// back to standard library.
type DefaultExecutor struct{}

// Exec proxies all requests to exec.Command()
// Used to support unit testing.
func (DefaultExecutor) Exec(cmd string, args []string) ([]byte, error) {
	log.Printf("Helper.Executor: executing command: %s %s", cmd, strings.Join(args, " "))
	cmdObj := exec.Command(cmd, args...)
	out, err := cmdObj.CombinedOutput()
	return out, err
}
