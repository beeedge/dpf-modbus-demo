/*
Copyright 2022 The BeeThings Authors.

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
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	ds "github.com/beeedge/beethings/pkg/device-access/rest/models"
	"github.com/beeedge/device-plugin-framework/shared"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"gopkg.in/yaml.v2"
)

// Here is a real implementation of device-plugin.
type Converter struct {
	logger           hclog.Logger
	// ConfigMaps For fast searching
	ModelIdMap       map[string]ds.Model
	DeviceIdMap      map[string]ds.Device
	FeatureIdMap     map[string]ds.Feature
	InputParamIdMap  map[string]ds.Param
	OutputParamIdMap map[string]ds.Param
}

// ConvertReportMessage2Devices converts data report request to protocol that device understands for each device of this device model,
func (c *Converter) ConvertReportMessage2Devices(modelId, featureId string) ([]string, error) {
	// TODO: concrete implement
	return []string{"Have a good try!!!"}, nil
}

// ConvertIssueMessage2Device converts issue request to protocol that device understands, which has four return parameters:
// 1. inputMessages: device issue protocols for each of command input param.
// 2. outputMessages: device data report protocols for each of command output param.
// 3. issueTopic: device issue MQTT topic for input params.
// 4. issueResponseTopic: device issue response MQ topic for output params.
func (c *Converter) ConvertIssueMessage2Device(deviceId, modelId, featureId string, values map[string]string) ([]string, []string, string, string, error) {
	if values != nil {
		for _, value := range values {
			switch c.InputParamIdMap[featureId].RegistryType {
			// Single holding registry length is 16bit, so first need to convert the values to multiple of 16 bit.
			// If len of value longer than num of holding registry * 16 bits, then keep the values shorter than num of holding registry * 16 bits.
			// If len of value shorter than num of holding registry * 16 bits, compensation zero to reach num of holding registry * 16 bits.
			// Coil registry length is 8bit, and others as the same as holding registry.
			// Here is a example explain how it works.
			case "holding registry":
				bytes := make([]byte, c.InputParamIdMap[featureId].RegistryNum*2)
				for i := 0; i < int(c.InputParamIdMap[featureId].RegistryNum*2); i++ {
					if 2*(i+1)-1 < len(value) {
						b := value[2*i : 2*(i+1)]
						v, err := strconv.ParseUint(b, 10, 16)
						if err != nil {
							return nil, nil, "", "", err
						}
						bytes[i] = uint8(v)
					}
				}
				return []string{string(bytes)}, nil, "", "", nil
			case "coil":
				bytes := make([]byte, c.InputParamIdMap[featureId].RegistryNum)
				for i := 0; i < int(c.InputParamIdMap[featureId].RegistryNum); i++ {
					if 2*(i+1)-1 < len(value) {
						b := value[2*i : 2*(i+1)]
						v, err := strconv.ParseUint(b, 10, 16)
						if err != nil {
							return nil, nil, "", "", err
						}
						bytes[i] = uint8(v)
					}
				}
				return []string{string(bytes)}, nil, "", "", nil
			}
		}
	}
	return nil, nil, "", "", fmt.Errorf("No any messages.")
}

// ConvertDeviceMessages2MQFormat receives device command issue responses and converts it to RabbitMQ normative format.
func (c *Converter) ConvertDeviceMessages2MQFormat(messages []string, featureType string) (string, []byte, error) {
	// Coil registry length is 8bit, so the length is not enough to convert by binary.BigEndian.Uint16(bytes). So we need to compensation zero to make it to 16bit.
	// Here is a example explain how it works.
	if messages != nil && len(messages[0]) > 0 {
		bytes := []byte(messages[0])
		if len(messages[0]) == 1 {
			bytes = append(bytes, []byte{0}[0])
		}
		d := binary.BigEndian.Uint16(bytes)
		data := strconv.FormatUint(uint64(d), 16)
		return "", []byte(data), nil
	}
	return "", nil, fmt.Errorf("No any messages.")
}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.Trace,
		Output:     os.Stderr,
		JSONFormat: true,
	})
	config, err := loadConfig(os.Getenv("PROTOCOL_CONFIG_PATH"))
	if err != nil {
		return
	}
	deviceIdMap := make(map[string]ds.Device)
	modelIdMap := make(map[string]ds.Model)
	featureIdMap := make(map[string]ds.Feature)
	inputParamIdMap := make(map[string]ds.Param)
	outputParamIdMap := make(map[string]ds.Param)
	for _, d := range config.Devices {
		logger.Info("deviceId = %s", d.DeviceId)
		deviceIdMap[d.DeviceId] = d
	}
	for _, m := range config.Models {
		for _, f := range m.Features {
			logger.Info("featureId = %s", f.Id)
			featureIdMap[f.Id] = f
			if f.Type == "command" {
				for _, in := range f.InputParams {
					logger.Info("input = %s", in.Id)
					inputParamIdMap[in.Id] = in
				}
				for _, out := range f.OutputParams {
					logger.Info("output = %s", out.Id)
					outputParamIdMap[out.Id] = out
				}
			}
		}
		logger.Info("modelId = %s", m.ModelId)
		modelIdMap[m.ModelId] = m
	}
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"converter": &shared.ConverterPlugin{Impl: &Converter{
				logger:           logger,
				ModelIdMap:       modelIdMap,
				DeviceIdMap:      deviceIdMap,
				FeatureIdMap:     featureIdMap,
				InputParamIdMap:  inputParamIdMap,
				OutputParamIdMap: outputParamIdMap,
			}},
		},

		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

func loadConfig(path string) (*ds.Protocol, error) {
	c := &ds.Protocol{}
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to read configuration file %s, error %s", path, err)
	}
	if err = yaml.Unmarshal(contents, c); err != nil {
		return nil, fmt.Errorf("Failed to parse configuration, error %s", err)
	}
	return c, nil
}
