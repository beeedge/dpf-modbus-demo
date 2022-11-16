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
	"os"
	"strconv"

	"github.com/beeedge/beethings/pkg/device-access/rest/models"
	"github.com/beeedge/device-plugin-framework/shared"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"gopkg.in/yaml.v2"
)

// Here is a real implementation of device-plugin.
type Converter struct {
	logger hclog.Logger
}

// ConvertIssueMessage2Device converts issue request to protocol that device understands, which has four return parameters:
// 1. inputMessages: device issue protocols for each of command input param.
// 2. outputMessages: device data report protocols for each of command output param.
// 3. issueTopic: device issue MQTT topic for input params.
// 4. issueResponseTopic: device issue response MQ topic for output params.
func (c *Converter) ConvertIssueMessage2Device(deviceId, modelId, featureId string, values map[string]string, convertedDeviceFeatureMap string) ([]string, []string, string, string, error) {
	var deviceFeatureMap models.DeviceFeatureMap
	if err := yaml.Unmarshal([]byte(convertedDeviceFeatureMap), &deviceFeatureMap); err != nil {
		c.logger.Info("Unmarshal convertedDeviceFeatureMap error: %s\n", err.Error())
		return nil, nil, "", "", err
	}
	c.logger.Info("value = %#v\n", values)
	c.logger.Info("deviceFeatureMap = %#v\n", deviceFeatureMap)
	if values != nil {
		for k, value := range values {
			c.logger.Info("k = %s\n", k)
			switch deviceFeatureMap.InputParamIdMap[k].RegisterType {
			// Single holding registry length is 16bit, so first need to convert the values to multiple of 16 bit.
			// If len of value longer than num of holding registry * 16 bits, then keep the values shorter than num of holding registry * 16 bits.
			// If len of value shorter than num of holding registry * 16 bits, compensation zero to reach num of holding registry * 16 bits.
			// Coil registry length is 8bit, and others as the same as holding registry.
			// Here is a example explain how it works.
			case models.ModbusRegisterTypeHolding:
				c.logger.Info("type = %s\n", models.ModbusRegisterTypeHolding)
				bytes := make([]byte, deviceFeatureMap.InputParamIdMap[k].RegisterNum*2)
				for i := 0; i < int(deviceFeatureMap.InputParamIdMap[k].RegisterNum*2); i++ {
					if 2*(i+1)-1 < len(value) {
						b := value[2*i : 2*(i+1)]
						v, err := strconv.ParseUint(b, 10, 16)
						if err != nil {
							return nil, nil, "", "", err
						}
						bytes[i] = uint8(v)
					}
				}
				c.logger.Info("bytes holding = %s\n", string(bytes))
				return []string{string(bytes)}, nil, "", "", nil
			case models.ModbusRegisterTypeCoil:
				c.logger.Info("type = %s\n", models.ModbusRegisterTypeCoil)
				bytes := make([]byte, deviceFeatureMap.InputParamIdMap[k].RegisterNum)
				for i := 0; i < int(deviceFeatureMap.InputParamIdMap[k].RegisterNum); i++ {
					if 2*(i+1)-1 < len(value) {
						b := value[2*i : 2*(i+1)]
						v, err := strconv.ParseUint(b, 10, 16)
						if err != nil {
							return nil, nil, "", "", err
						}
						bytes[i] = uint8(v)
					}
				}
				c.logger.Info("bytes coil = %s\n", string(bytes))
				return []string{string(bytes)}, nil, "", "", nil
			}
		}
	}
	return nil, nil, "", "", fmt.Errorf("No any messages.")
}

// ConvertDeviceMessages2MQFormat receives device command issue responses and converts it to RabbitMQ normative format.
func (c *Converter) ConvertDeviceMessages2MQFormat(messages []string, convertedDeviceFeatureMap string) (string, []byte, error) {
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
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"converter": &shared.ConverterPlugin{Impl: &Converter{
				logger: logger,
			}},
		},
		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
