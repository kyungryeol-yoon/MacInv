package paths

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed data/vendors.json
var vendorsJSON []byte

// Vendor는 한 벤더의 폴더/앱/공유 컴포넌트 매핑이다(data/vendors.json 스키마).
type Vendor struct {
	Name             string            `json:"vendor"`
	BundlePrefixes   []string          `json:"bundle_prefixes"`
	Folders          []string          `json:"folders"` // Library 루트들 아래 벤더 폴더 이름
	Apps             map[string]string `json:"apps"`    // bundleID -> 벤더 폴더 안 앱 하위 폴더 이름
	SharedComponents []SharedComponent `json:"shared_components"`
}

// SharedComponent는 벤더 앱들이 공유하는 컴포넌트(Keystone 등)다.
type SharedComponent struct {
	Name  string   `json:"name"`
	Globs []string `json:"globs"` // home 기준 상대 glob
}

type vendorTable struct {
	Vendors []Vendor `json:"vendors"`
}

var (
	vendorOnce  sync.Once
	vendorCache vendorTable
)

func Vendors() []Vendor {
	vendorOnce.Do(func() {
		_ = json.Unmarshal(vendorsJSON, &vendorCache) // 임베드된 데이터라 실패할 일 없음
	})
	return vendorCache.Vendors
}

// LookupVendor는 Bundle ID로 벤더와 앱 하위 폴더 이름을 찾는다(순수 함수 — 테스트 대상).
// 앱이 apps 맵에 없으면 appName에서 벤더명을 떼어 하위 폴더를 추정한다.
func LookupVendor(vs []Vendor, bundleID, appName string) (v Vendor, appSubdir string, ok bool) {
	for _, cand := range vs {
		for _, pfx := range cand.BundlePrefixes {
			if strings.HasPrefix(bundleID, pfx) {
				sub := cand.Apps[bundleID]
				if sub == "" {
					sub = deriveSubdir(cand.Name, appName)
				}
				return cand, sub, true
			}
		}
	}
	return Vendor{}, "", false
}

// deriveSubdir는 "Google Chrome" 같은 앱 이름에서 벤더명("Google")을 떼어 "Chrome"을 얻는다.
func deriveSubdir(vendor, appName string) string {
	name := strings.TrimSuffix(appName, ".app")
	trimmed := strings.TrimSpace(strings.TrimPrefix(name, vendor))
	if trimmed == "" {
		return name
	}
	return trimmed
}

// CountVendorApps는 설치된 Bundle ID들 중 벤더 접두사에 맞는 개수를 센다(exclude는 대상 자신).
func CountVendorApps(v Vendor, installedIDs []string, exclude string) int {
	n := 0
	for _, id := range installedIDs {
		if id == exclude {
			continue
		}
		for _, pfx := range v.BundlePrefixes {
			if strings.HasPrefix(id, pfx) {
				n++
				break
			}
		}
	}
	return n
}
