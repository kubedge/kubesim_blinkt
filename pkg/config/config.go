/*
Copyright 2018 Kubedge

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

package config

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
)

type BlinktConfigData struct {
	Algorithm string `yaml:"algorithm"`
	Intensity int    `yaml:"intensity"`
	Frequency int    `yaml:"frequency"`
	Pixel0    []int  `yaml:"pixel0"`
	Pixel1    []int  `yaml:"pixel1"`
	Pixel2    []int  `yaml:"pixel2"`
	Pixel3    []int  `yaml:"pixel3"`
	Pixel4    []int  `yaml:"pixel4"`
	Pixel5    []int  `yaml:"pixel5"`
	Pixel6    []int  `yaml:"pixel6"`
	Pixel7    []int  `yaml:"pixel7"`
}

func (config *BlinktConfigData) Config() {

	yamlFile, err := ioutil.ReadFile("/etc/kubedge/blinkt_conf.yaml")
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}

	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
}
