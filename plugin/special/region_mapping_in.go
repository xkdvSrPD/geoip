package special

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Loyalsoldier/geoip/lib"
)

const (
	TypeRegionMappingIn = "regionMapping"
	DescRegionMappingIn = "Convert region mapping to other formats"
)

func init() {
	lib.RegisterInputConfigCreator(TypeRegionMappingIn, func(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
		return newRegionMappingIn(action, data)
	})
	lib.RegisterInputConverter(TypeRegionMappingIn, &RegionMappingIn{
		Description: DescRegionMappingIn,
	})
}

func newRegionMappingIn(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		URI        string     `json:"uri"`
		Want       []string   `json:"wantedList"`
		OnlyIPType lib.IPType `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if tmp.URI == "" {
		tmp.URI = "./region_mapping.json"
	}

	// Filter want list
	wantList := make(map[string]bool)
	for _, want := range tmp.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" {
			wantList[want] = true
		}
	}

	return &RegionMappingIn{
		Type:        TypeRegionMappingIn,
		Action:      action,
		Description: DescRegionMappingIn,
		URI:         tmp.URI,
		Want:        wantList,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}

type RegionMappingIn struct {
	Type        string
	Action      lib.Action
	Description string
	URI         string
	Want        map[string]bool
	OnlyIPType  lib.IPType
}

func (r *RegionMappingIn) GetType() string {
	return r.Type
}

func (r *RegionMappingIn) GetAction() lib.Action {
	return r.Action
}

func (r *RegionMappingIn) GetDescription() string {
	return r.Description
}

func (r *RegionMappingIn) Input(container lib.Container) (lib.Container, error) {
	// Read region mapping file
	content, err := os.ReadFile(r.URI)
	if err != nil {
		return nil, fmt.Errorf("❌ [type %s | action %s] failed to read region mapping file: %v", r.Type, r.Action, err)
	}

	// Parse JSON
	var regionMapping map[string][]string
	if err := json.Unmarshal(content, &regionMapping); err != nil {
		return nil, fmt.Errorf("❌ [type %s | action %s] failed to parse region mapping JSON: %v", r.Type, r.Action, err)
	}

	// Build a map of all entries for faster lookup
	entryMap := make(map[string]*lib.Entry)
	for entry := range container.Loop() {
		entryMap[entry.GetName()] = entry
	}

	// Process each region
	for regionName, countryCodes := range regionMapping {
		regionName = strings.ToUpper(strings.TrimSpace(regionName))
		
		// Check if this region is wanted
		if len(r.Want) > 0 && !r.Want[regionName] {
			continue
		}

		// Create the region entry
		regionEntry := lib.NewEntry(regionName)

		// Merge IP addresses from all countries in this region
		for _, countryCode := range countryCodes {
			countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
			if countryCode == "" {
				continue
			}

			// Get the country entry from map
			if entry, exists := entryMap[countryCode]; exists {
				// Copy IP addresses from country to region
				var entryCidr []string
				switch r.OnlyIPType {
				case lib.IPv4:
					entryCidr, err = entry.MarshalText(lib.IgnoreIPv6)
				case lib.IPv6:
					entryCidr, err = entry.MarshalText(lib.IgnoreIPv4)
				default:
					entryCidr, err = entry.MarshalText()
				}
				if err != nil {
					return nil, fmt.Errorf("❌ [type %s | action %s] failed to marshal country %s: %v", r.Type, r.Action, countryCode, err)
				}

				// Add each CIDR to the region entry
				for _, cidr := range entryCidr {
					if err := regionEntry.AddPrefix(cidr); err != nil {
						return nil, fmt.Errorf("❌ [type %s | action %s] failed to add CIDR %s to region %s: %v", r.Type, r.Action, cidr, regionName, err)
					}
				}
			}
		}

		// Add the region entry to container
		switch r.Action {
		case lib.ActionAdd:
			if err := container.Add(regionEntry); err != nil {
				return nil, fmt.Errorf("❌ [type %s | action %s] failed to add region %s: %v", r.Type, r.Action, regionName, err)
			}
		case lib.ActionRemove:
			var ignoreIPType lib.IgnoreIPOption
			switch r.OnlyIPType {
			case lib.IPv4:
				ignoreIPType = lib.IgnoreIPv6
			case lib.IPv6:
				ignoreIPType = lib.IgnoreIPv4
			}
			if err := container.Remove(regionEntry, lib.CaseRemoveEntry, ignoreIPType); err != nil {
				return nil, fmt.Errorf("❌ [type %s | action %s] failed to remove region %s: %v", r.Type, r.Action, regionName, err)
			}
		}
	}

	return container, nil
}