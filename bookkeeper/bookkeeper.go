// Copyright (c) 2019 IoTeX
// This program is free software: you can redistribute it and/or modify it under the terms of the
// GNU General Public License as published by the Free Software Foundation, either version 3 of
// the License, or (at your option) any later version.
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See
// the GNU General Public License for more details.
// You should have received a copy of the GNU General Public License along with this program. If
// not, see <http://www.gnu.org/licenses/>.

package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/iotexproject/iotex-tools/bookkeeper/cmd"
)

const (
	// Disclaim defines the disclaim of this software
	Disclaim = "This Bookkeeper is a REFERENCE IMPLEMENTATION of reward distribution tool provided by IOTEX FOUNDATION. IOTEX FOUNDATION disclaims all responsibility for any damages or losses (including, without limitation, financial loss, damages for loss in business projects, loss of profits or other consequential losses) arising in contract, tort or otherwise from the use of or inability to use the Bookkeeper, or from any action or decision taken as a result of using this Bookkeeper."
)

func init() {
	// init zap
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	zapCfg.Level.SetLevel(zap.WarnLevel)
	l, err := zapCfg.Build()
	if err != nil {
		log.Fatalln("Failed to init zap global logger, no zap log will be shown till zap is properly initialized: ", err)
	}
	zap.ReplaceGlobals(l)

	RootCmd.AddCommand(cmd.ConvertCmd)
	RootCmd.AddCommand(cmd.ExportCmd)
}

var RootCmd = &cobra.Command{
	Use:   "bookkeeper",
	Short: "tool to export rewards",
	Long:  "bookkeeper is a command line based tool to export rewards by block producer name",
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
