/*
Copyright (c) 2025 Vates

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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"

	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"

	"k8s.io/klog/v2"
)

func init() {
	klog.InitFlags(nil)
	driverOptions.AddFlags().VisitAll(func(f *flag.Flag) {
		flag.CommandLine.Var(f.Value, f.Name, f.Usage)
	})
}

var (
	showVersion   = flag.Bool("version", false, "Print the version and exit.")
	driverOptions xenorchestracsi.DriverOptions
)

func main() {
	flag.Parse()
	if *showVersion {
		baseName := path.Base(os.Args[0])
		fmt.Printf("%s - %s\n", baseName, xenorchestracsi.GetVersionInfo())
		return
	}

	driver := xenorchestracsi.NewDriver(&driverOptions)
	if driver == nil {
		klog.Fatalln("Failed to initialize Xen Orchestra CSI Driver")
	}
	if err := driver.Run(context.Background()); err != nil {
		klog.Fatalf("Failed to run Xen Orchestra CSI Driver: %v", err)
	}
	os.Exit(0)
}
