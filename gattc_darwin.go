package bluetooth

import (
	"errors"
	"time"

	"github.com/JuulLabs-OSS/cbgo"
)

// DiscoverServices starts a service discovery procedure. Pass a list of service
// UUIDs you are interested in to this function. Either a slice of all services
// is returned (of the same length as the requested UUIDs and in the same
// order), or if some services could not be discovered an error is returned.
//
// Passing a nil slice of UUIDs will return a complete list of
// services.
func (d *Device) DiscoverServices(uuids []UUID) ([]*DeviceService, error) {
	cbuuids := []cbgo.UUID{}
	for _, u := range uuids {
		uuid, _ := cbgo.ParseUUID(u.String())
		cbuuids = append(cbuuids, uuid)
	}

	d.prph.DiscoverServices(cbuuids)

	// clear cache of services
	d.services = make(map[UUID]*DeviceService)

	// wait on channel for service discovery
	select {
	case <-d.servicesChan:
		svcs := []*DeviceService{}
		for _, dsvc := range d.prph.Services() {
			uuid, _ := ParseUUID(dsvc.UUID().String())
			svc := &DeviceService{
				uuidWrapper: uuid,
				device:      d,
				service:     dsvc,
			}
			svcs = append(svcs, svc)
			d.services[svc.uuidWrapper] = svc
		}
		return svcs, nil
	case <-time.NewTimer(10 * time.Second).C:
		return nil, errors.New("timeout on DiscoverServices")
	}
}

// uuidWrapper is a type alias for UUID so we ensure no conflicts with
// struct method of the same name.
type uuidWrapper = UUID

// DeviceService is a BLE service on a connected peripheral device.
type DeviceService struct {
	uuidWrapper

	device *Device

	service cbgo.Service
}

// UUID returns the UUID for this DeviceService.
func (s *DeviceService) UUID() UUID {
	return s.uuidWrapper
}

// DiscoverCharacteristics discovers characteristics in this service. Pass a
// list of characteristic UUIDs you are interested in to this function. Either a
// list of all requested services is returned, or if some services could not be
// discovered an error is returned. If there is no error, the characteristics
// slice has the same length as the UUID slice with characteristics in the same
// order in the slice as in the requested UUID list.
//
// Passing a nil slice of UUIDs will return a complete list of
// characteristics.
func (s *DeviceService) DiscoverCharacteristics(uuids []UUID) ([]*DeviceCharacteristic, error) {
	cbuuids := []cbgo.UUID{}
	for _, u := range uuids {
		uuid, _ := cbgo.ParseUUID(u.String())
		cbuuids = append(cbuuids, uuid)
	}

	s.device.prph.DiscoverCharacteristics(cbuuids, s.service)

	// clear cache of characteristics
	s.device.characteristics = make(map[UUID]*DeviceCharacteristic)

	// wait on channel for characteristic discovery
	select {
	case <-s.device.charsChan:
		chars := []*DeviceCharacteristic{}
		for _, dchar := range s.service.Characteristics() {
			uuid, _ := ParseUUID(dchar.UUID().String())
			char := &DeviceCharacteristic{
				uuidWrapper:    uuid,
				service:        s,
				characteristic: dchar,
			}
			chars = append(chars, char)
			s.device.characteristics[char.uuidWrapper] = char
		}
		return chars, nil
	case <-time.NewTimer(10 * time.Second).C:
		return nil, errors.New("timeout on DiscoverCharacteristics")
	}
}

// DeviceCharacteristic is a BLE characteristic on a connected peripheral
// device.
type DeviceCharacteristic struct {
	uuidWrapper

	service *DeviceService

	characteristic cbgo.Characteristic
	callback       func(buf []byte)
	readChan       chan error
}

// UUID returns the UUID for this DeviceCharacteristic.
func (c *DeviceCharacteristic) UUID() UUID {
	return c.uuidWrapper
}

// WriteWithoutResponse replaces the characteristic value with a new value. The
// call will return before all data has been written. A limited number of such
// writes can be in flight at any given time. This call is also known as a
// "write command" (as opposed to a write request).
func (c *DeviceCharacteristic) WriteWithoutResponse(p []byte) (n int, err error) {
	c.service.device.prph.WriteCharacteristic(p, c.characteristic, false)

	return len(p), nil
}

// EnableNotifications enables notifications in the Client Characteristic
// Configuration Descriptor (CCCD). This means that most peripherals will send a
// notification with a new value every time the value of the characteristic
// changes.
func (c *DeviceCharacteristic) EnableNotifications(callback func(buf []byte)) error {
	if callback == nil {
		return errors.New("must provide a callback for EnableNotifications")
	}

	c.callback = callback
	c.service.device.prph.SetNotify(true, c.characteristic)

	return nil
}

// Read reads the current characteristic value.
func (c *DeviceCharacteristic) Read() (data []byte, err error) {
	c.readChan = make(chan error)
	c.service.device.prph.ReadCharacteristic(c.characteristic)

	// wait for result
	select {
	case <-c.readChan:
		c.readChan = nil
	case <-time.NewTimer(10 * time.Second).C:
		c.readChan = nil
		return nil, errors.New("timeout on Read()")
	}

	return c.characteristic.Value(), nil
}
