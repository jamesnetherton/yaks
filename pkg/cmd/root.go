/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"os"

	"github.com/jboss-fuse/yaks/pkg/client"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const yaksCommandLongDescription = `Yaks is Yet Another Kamel Subproject.
`

// RootCmdOptions --
type RootCmdOptions struct {
	Context    context.Context
	_client    client.Client
	KubeConfig string
	Namespace  string
}

// NewYaksCommand --
func NewYaksCommand(ctx context.Context) (*cobra.Command, error) {
	options := RootCmdOptions{
		Context: ctx,
	}
	var cmd = cobra.Command{
		PersistentPreRunE: options.preRun,
		Use:               "yaks",
		Short:             "Yaks is a awesome client tool for running tests natively on Kubernetes",
		Long:              yaksCommandLongDescription,
	}

	cmd.PersistentFlags().StringVar(&options.KubeConfig, "config", os.Getenv("KUBECONFIG"), "Path to the config file to use for CLI requests")
	cmd.PersistentFlags().StringVarP(&options.Namespace, "namespace", "n", "", "Namespace to use for all operations")

	cmd.AddCommand(newCmdTest(&options))
	cmd.AddCommand(newCmdInstall(&options))

	return &cmd, nil
}

func (command *RootCmdOptions) preRun(cmd *cobra.Command, _ []string) error {
	if command.Namespace == "" {
		current, err := client.GetCurrentNamespace(command.KubeConfig)
		if err != nil {
			return errors.Wrap(err, "cannot get current namespace")
		}
		err = cmd.Flag("namespace").Value.Set(current)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetCmdClient returns the client that can be used from command line tools
func (command *RootCmdOptions) GetCmdClient() (client.Client, error) {
	// Get the pre-computed client
	if command._client != nil {
		return command._client, nil
	}
	var err error
	command._client, err = command.NewCmdClient()
	return command._client, err
}

// NewCmdClient returns a new client that can be used from command line tools
func (command *RootCmdOptions) NewCmdClient() (client.Client, error) {
	return client.NewOutOfClusterClient(command.KubeConfig)
}
