package mtp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ghp3000/usb"
)

type Filter func(*Device) bool

func candidateFromDeviceDescriptor(d *usb.Device) *Device {
	dd, err := d.GetDeviceDescriptor()
	if err != nil {
		return nil
	}
	for i := byte(0); i < dd.NumConfigurations; i++ {
		cdecs, err := d.GetConfigDescriptor(i)
		if err != nil {
			return nil
		}
		for _, iface := range cdecs.Interfaces {
			for _, a := range iface.AltSetting {
				if len(a.EndPoints) != 3 {
					continue
				}
				m := Device{}
				for _, s := range a.EndPoints {
					switch {
					case s.Direction() == usb.ENDPOINT_IN && s.TransferType() == usb.TRANSFER_TYPE_INTERRUPT:
						m.eventEP = s.EndpointAddress
					case s.Direction() == usb.ENDPOINT_IN && s.TransferType() == usb.TRANSFER_TYPE_BULK:
						m.fetchEP = s.EndpointAddress
					case s.Direction() == usb.ENDPOINT_OUT && s.TransferType() == usb.TRANSFER_TYPE_BULK:
						m.sendEP = s.EndpointAddress
					}
				}
				if m.sendEP > 0 && m.fetchEP > 0 && m.eventEP > 0 {
					m.devDescr = *dd
					m.ifaceDescr = a
					m.dev = d.Ref()
					m.configValue = cdecs.ConfigurationValue
					return &m
				}
			}
		}
	}

	return nil
}

// FindDevices finds likely MTP devices without opening them.
func FindDevices(c *usb.Context) ([]*Device, error) {
	l, err := c.GetDeviceList()
	if err != nil {
		return nil, err
	}

	var cands []*Device
	for _, d := range l {
		busNumber := d.GetBusNumber()
		paths := d.GetDevicePath()
		//if path != "" && makePathString(busNumber, paths) != path {
		//	continue
		//}
		cand := candidateFromDeviceDescriptor(d)
		if cand != nil {
			cand.busNum = busNumber
			cand.path = paths
			cands = append(cands, cand)
		}
	}

	if len(l) > 0 {
		l.Done()
	}

	return cands, nil
}
func makePathString(bus uint8, path []int) string {
	str := strconv.Itoa(int(bus))
	for i := 0; i < len(path); i++ {
		if i == 0 {
			str = str + "-" + strconv.Itoa(path[i])
		} else {
			str = str + "." + strconv.Itoa(path[i])
		}
	}
	return str
}

// selectDevice finds a device that matches given pattern
func selectDevice(cands []*Device, pattern string) (*Device, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var found []*Device
	for _, cand := range cands {
		if err = cand.Open(); err != nil {
			continue
		}
		found = append(found, cand)
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no MTP devices found")
	}

	cands = found
	found = nil
	var ids []string
	for i, cand := range cands {
		id, err := cand.ID()
		if err != nil {
			// TODO - close cands
			return nil, fmt.Errorf("Id dev %d: %v", i, err)
		}

		if pattern == "" || re.FindString(id) != "" {
			found = append(found, cand)
			ids = append(ids, id)
		} else {
			cand.Close()
			cand.Done()
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no device matched")
	}

	if len(found) > 1 {
		for _, cand := range found {
			cand.Close()
			cand.Done()
		}
		return nil, fmt.Errorf("mtp: more than 1 device: %s", strings.Join(ids, ","))
	}

	cand := found[0]
	config, err := cand.h.GetConfiguration()
	if err != nil {
		return nil, fmt.Errorf("could not get configuration of %v: %v",
			ids[0], err)
	}
	if config != cand.configValue {

		if err := cand.h.SetConfiguration(cand.configValue); err != nil {
			return nil, fmt.Errorf("could not set configuration of %v: %v",
				ids[0], err)
		}
	}
	return found[0], nil
}

func selectDeviceByFilter(cands []*Device, filter Filter, debug bool) (*Device, error) {
	var found []*Device
	for _, cand := range cands {
		if err := cand.Open(); err != nil {
			continue
		}
		found = append(found, cand)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no MTP devices found")
	}

	cands = found
	found = nil
	for _, cand := range cands {
		if filter != nil {
			if filter(cand) {
				found = append(found, cand)
				continue
			}
		}
		cand.Close()
		cand.Done()
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no device matched")
	}

	if len(found) > 1 {
		for _, cand := range found {
			cand.Close()
			cand.Done()
		}
		return nil, fmt.Errorf("mtp: more than 1 device")
	}

	cand := found[0]
	config, err := cand.h.GetConfiguration()
	if err != nil {
		return nil, fmt.Errorf("could not get configuration, %v", err)
	}
	if config != cand.configValue {

		if err := cand.h.SetConfiguration(cand.configValue); err != nil {
			return nil, fmt.Errorf("could not set configuration of %v: %v",
				cand, err)
		}
	}
	return cand, nil
}

// SelectDevice returns opened MTP device that matches the given pattern.
func SelectDevice(pattern string, path string) (*Device, error) {
	c := usb.NewContext()

	devs, err := FindDevices(c)
	if err != nil {
		return nil, err
	}
	if len(devs) == 0 {
		return nil, fmt.Errorf("no MTP devices found")
	}

	return selectDevice(devs, pattern)
}
func SelectDeviceByFilter(filter Filter, debug bool) (*Device, error) {
	c := usb.NewContext()

	devs, err := FindDevices(c)
	if err != nil {
		return nil, err
	}
	if len(devs) == 0 {
		return nil, fmt.Errorf("no MTP devices found")
	}

	return selectDeviceByFilter(devs, filter, debug)
}

// SelectDeviceForDebugging returns opened MTP device that matches the given pattern and debug information are set true
func SelectDeviceWithDebugging(pattern string, allowDebugging bool) (*Device, error) {
	c := usb.NewContext()

	devs, err := FindDevices(c)
	if err != nil {
		return nil, err
	}
	if len(devs) == 0 {
		return nil, fmt.Errorf("no MTP devices found")
	}

	if allowDebugging {
		for _, _dev := range devs {
			_dev.USBDebug = allowDebugging
			_dev.DataDebug = allowDebugging
			_dev.MTPDebug = allowDebugging
		}
	}

	return selectDevice(devs, pattern)
}
