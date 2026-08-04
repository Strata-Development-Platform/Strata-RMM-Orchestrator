package probe

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"
)

// IPMIPayload represents an IPMI network payload.
type IPMIPayload struct {
	NetFn    byte
	RsLun    byte
	RsSsn    byte
	Seq      byte
	Checksum byte
	Address  byte
	Request  []byte
}

// IPMIResult represents the result of an IPMI query.
type IPMIResult struct {
	DeviceIP  string                 `json:"device_ip"`
	Port      int                    `json:"port"`
	Channel   int                    `json:"channel"`
	Status    string                 `json:"status"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// IPMITarget is an IPMI-capable device (BMC, CIMC, etc.).
type IPMITarget struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`       // Default 623
	Channel   int    `json:"channel"`    // Default 0
	Username  string `json:"username"`
	Password  string `json:"password"`
	AuthType  string `json:"auth_type"` // md5, sha, none
	Timeout   int    `json:"timeout"`   // seconds
}

// IPMIClient communicates with IPMI devices.
type IPMIClient struct {
	target  IPMITarget
	conn    *net.UDPConn
	timeout time.Duration
}

// NewIPMIClient creates a new IPMI client.
func NewIPMIClient(target IPMITarget) *IPMIClient {
	if target.Port == 0 {
		target.Port = 623
	}
	if target.Channel == 0 {
		target.Channel = 0
	}
	if target.Timeout == 0 {
		target.Timeout = 10
	}

	return &IPMIClient{
		target:  target,
		timeout: time.Duration(target.Timeout) * time.Second,
	}
}

// Connect establishes a UDP connection to the IPMI device.
func (c *IPMIClient) Connect(ctx context.Context) error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(c.target.Host),
		Port: c.target.Port,
	}
	if addr.IP == nil {
		return fmt.Errorf("invalid IP address: %s", c.target.Host)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("connect to IPMI device: %w", err)
	}
	c.conn = conn
	return nil
}

// Close closes the IPMI connection.
func (c *IPMIClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

// SensorData retrieves sensor data from the IPMI device.
func (c *IPMIClient) SensorData(ctx context.Context) (*IPMIResult, error) {
	if err := c.Connect(ctx); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}
	defer c.Close()

	// IPMI Sensor Reading Request (NetFn = 0x0a, Request = 0x2d)
	payload := c.buildSensorRequest()
	if _, err := c.conn.Write(payload); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	resp, err := c.readResponse()
	if err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	return &IPMIResult{
		DeviceIP:  c.target.Host,
		Port:      c.target.Port,
		Channel:   c.target.Channel,
		Status:    "ok",
		Data:      c.parseSensorResponse(resp),
		Timestamp: time.Now().UTC(),
	}, nil
}

// ChassisStatus retrieves chassis power and system status.
func (c *IPMIClient) ChassisStatus(ctx context.Context) (*IPMIResult, error) {
	if err := c.Connect(ctx); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}
	defer c.Close()

	// Chassis Status Command (NetFn = 0x0a, Request = 0x01)
	payload := c.buildChassisStatusRequest()
	if _, err := c.conn.Write(payload); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	resp, err := c.readResponse()
	if err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	return &IPMIResult{
		DeviceIP:  c.target.Host,
		Port:      c.target.Port,
		Channel:   c.target.Channel,
		Status:    "ok",
		Data:      c.parseChassisResponse(resp),
		Timestamp: time.Now().UTC(),
	}, nil
}

// FRUData retrieves FRU (Field Replaceable Unit) information.
func (c *IPMIClient) FRUData(ctx context.Context) (*IPMIResult, error) {
	if err := c.Connect(ctx); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}
	defer c.Close()

	// FRU Device Read Command (NetFn = 0x0a, Request = 0x39)
	payload := c.buildFRURequest()
	if _, err := c.conn.Write(payload); err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	resp, err := c.readResponse()
	if err != nil {
		return &IPMIResult{
			DeviceIP:  c.target.Host,
			Port:      c.target.Port,
			Channel:   c.target.Channel,
			Status:    "error",
			Error:     err.Error(),
			Timestamp: time.Now().UTC(),
		}, nil
	}

	return &IPMIResult{
		DeviceIP:  c.target.Host,
		Port:      c.target.Port,
		Channel:   c.target.Channel,
		Status:    "ok",
		Data:      c.parseFRUResponse(resp),
		Timestamp: time.Now().UTC(),
	}, nil
}

// BuildSensorRequest creates an IPMI Sensor Reading Request payload.
func (c *IPMIClient) buildSensorRequest() []byte {
	return []byte{
		0x0a, // NetFn (Storage)
		// #nosec G115
		byte(c.target.Channel << 4), // LUN
		0x00, // RS LUN
		0x00, // RS SSN
		0x00, // Sequence (not used for unicast)
		0x00, // Checksum
		0x0c, // Address type (Broadcast LUN)
		0x2d, // Request (Sensor Reading)
		0x00, // Number of bytes (optional)
	}
}

// BuildChassisStatusRequest creates an IPMI Chassis Status Request payload.
func (c *IPMIClient) buildChassisStatusRequest() []byte {
	return []byte{
		0x0a, // NetFn (Application)
		// #nosec G115
		byte(c.target.Channel << 4), // LUN
		0x00, // RS LUN
		0x00, // RS SSN
		0x00, // Sequence
		0x00, // Checksum
		0x0c, // Address type
		0x01, // Request (Chassis Status)
	}
}

// BuildFRURequest creates an IPMI FRU Device Read Request payload.
func (c *IPMIClient) buildFRURequest() []byte {
	return []byte{
		0x0a, // NetFn (Application)
		// #nosec G115
		byte(c.target.Channel << 4), // LUN
		0x00, // RS LUN
		0x00, // RS SSN
		0x00, // Sequence
		0x00, // Checksum
		0x0c, // Address type
		0x39, // Request (FRU Device Read)
		0x00, // FRU Device ID
		0x00, // Offset
		0x00, // Length
	}
}

// ReadResponse reads the response from the IPMI device.
func (c *IPMIClient) readResponse() ([]byte, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	buf := make([]byte, 1024)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return buf[:n], nil
}

// ParseSensorResponse parses IPMI sensor reading response data.
func (c *IPMIClient) parseSensorResponse(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	if len(data) < 4 {
		result["error"] = "response too short"
		return result
	}

	result["completion_code"] = data[0]
	result["sensor_id"] = hex.EncodeToString(data[1:3])
	result["event_reading_type"] = fmt.Sprintf("0x%02x", data[3])

	return result
}

// ParseChassisResponse parses IPMI chassis status response data.
func (c *IPMIClient) parseChassisResponse(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	if len(data) < 3 {
		result["error"] = "response too short"
		return result
	}

	result["completion_code"] = data[0]
	result["power_state"] = decodePowerState(data[1])
	result["system_state"] = decodeSystemState(data[2])

	return result
}

// ParseFRUResponse parses IPMI FRU device response data.
func (c *IPMIClient) parseFRUResponse(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	if len(data) < 2 {
		result["error"] = "response too short"
	} else {
		result["completion_code"] = data[0]
		result["fru_data"] = hex.EncodeToString(data[1:])
	}
	return result
}

// DecodePowerState decodes IPMI power state bytes.
func decodePowerState(byteVal byte) string {
	switch {
	case byteVal&0x01 != 0:
		return "on"
	case byteVal&0x02 != 0:
		return "off"
	case byteVal&0x04 != 0:
		return "powering_on"
	case byteVal&0x08 != 0:
		return "powering_off"
	default:
		return "unknown"
	}
}

// DecodeSystemState decodes IPMI system state bytes.
func decodeSystemState(byteVal byte) string {
	switch {
	case byteVal&0x01 != 0:
		return "running"
	case byteVal&0x02 != 0:
		return "booting"
	case byteVal&0x04 != 0:
		return "in_diagnostic_mode"
	case byteVal&0x08 != 0:
		return "paused"
	default:
		return "unknown"
	}
}

// DecodeSensorValue decodes an IPMI sensor value.
func DecodeSensorValue(sensorType byte, reading byte) string {
	switch sensorType {
	case 0x01: // Temperature
		return fmt.Sprintf("%d°C", reading)
	case 0x02: // Fan
		return fmt.Sprintf("%d RPM", reading)
	case 0x04: // Voltage
		return fmt.Sprintf("%.2fV", float64(reading)*0.001)
	case 0x05: // Current
		return fmt.Sprintf("%.2fA", float64(reading)*0.001)
	case 0x20: // Physical security
		return decodePhysicalSecurity(reading)
	case 0x21: // Progress
		return fmt.Sprintf("0x%02x", reading)
	default:
		return fmt.Sprintf("0x%02x", reading)
	}
}

func decodePhysicalSecurity(reading byte) string {
	if reading&0x01 != 0 {
		return "breach"
	}
	if reading&0x02 != 0 {
		return "unsafe"
	}
	return "safe"
}

// BuildSNMPTrap creates an IPMI-style trap payload for syslog forwarding.
func BuildSNMPTrap(deviceIP, message string) []byte {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(message))) // #nosec G115
	buf.WriteString(message)
	return buf.Bytes()
}
