package clean

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVendorsEmbedLoads(t *testing.T) {
	vs := vendors()
	if len(vs) == 0 {
		t.Fatal("임베드된 vendors.json이 비었음")
	}
	if _, _, ok := lookupVendor(vs, "com.google.Chrome", "Google Chrome"); !ok {
		t.Error("Google 벤더를 찾지 못함")
	}
}

func TestLookupVendor(t *testing.T) {
	vs := vendors()

	v, sub, ok := lookupVendor(vs, "com.google.Chrome", "Google Chrome")
	if !ok || v.Name != "Google" || sub != "Chrome" {
		t.Errorf("Chrome 매핑 오류: v=%s sub=%q ok=%v", v.Name, sub, ok)
	}
	// apps 맵에 없는 구글 앱은 이름에서 벤더명을 떼어 추정.
	_, sub2, ok2 := lookupVendor(vs, "com.google.SomethingNew", "Google Something")
	if !ok2 || sub2 != "Something" {
		t.Errorf("추정 하위폴더 오류: sub=%q ok=%v", sub2, ok2)
	}
	// 매칭 안 되는 것.
	if _, _, ok3 := lookupVendor(vs, "org.godotengine.godot", "Godot"); ok3 {
		t.Error("Godot이 벤더에 매칭되면 안 됨")
	}
}

func TestCountVendorApps(t *testing.T) {
	v, _, _ := lookupVendor(vendors(), "com.google.Chrome", "Google Chrome")
	installed := []string{"com.google.Chrome", "com.google.drivefs", "com.apple.Safari", "org.godotengine.godot"}
	if n := countVendorApps(v, installed, "com.google.Chrome"); n != 1 {
		t.Errorf("다른 구글 앱 수 = %d, want 1 (drivefs)", n)
	}
	if n := countVendorApps(v, []string{"com.google.Chrome"}, "com.google.Chrome"); n != 0 {
		t.Errorf("이 앱뿐인데 = %d, want 0", n)
	}
}

// Chrome 첫 테스트 케이스: 다른 구글 앱이 있으면 Google/Chrome 하위만 low, Keystone은 제외.
func TestAugmentVendorChromeSharedFolder(t *testing.T) {
	home := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	chromeDir := mk("Library/Application Support/Google/Chrome")
	mk("Library/Application Support/Google/DriveFS") // 다른 구글 앱 데이터
	keystone := mk("Library/Google/GoogleSoftwareUpdate")

	tgt := &Target{Kind: KindApp, Name: "Google Chrome.app", BundleID: "com.google.Chrome"}
	// 다른 구글 앱(DriveFS)이 설치돼 있는 상황.
	augmentVendor(tgt, home, "com.google.Chrome", "Google Chrome",
		func() []string { return []string{"com.google.Chrome", "com.google.drivefs"} })

	var gotChrome bool
	for _, p := range tgt.TrashPaths {
		if p.Path == chromeDir {
			gotChrome = true
			if p.Confidence != ConfLow {
				t.Errorf("Chrome 하위 폴더 확신도 = %s, want low", p.Confidence)
			}
		}
		if p.Path == keystone {
			t.Errorf("Keystone이 삭제 후보에 들어감(공유 컴포넌트여야 제외): %s", keystone)
		}
	}
	if !gotChrome {
		t.Errorf("Google/Chrome 하위 폴더가 후보에 없음. paths=%v", tgt.TrashPaths)
	}
	var keystoneExcluded bool
	for _, e := range tgt.Excluded {
		if e.Path == keystone {
			keystoneExcluded = true
		}
	}
	if !keystoneExcluded {
		t.Errorf("Keystone이 제외 목록에 없음. excluded=%v", tgt.Excluded)
	}
}

// Office: Word를 지우는데 Excel이 남아있으면 Word 하위만 low, 공유 라이선스/업데이터는 보존.
func TestAugmentVendorOfficeSharedLicense(t *testing.T) {
	home := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	wordDir := mk("Library/Application Support/Microsoft/Word")
	mk("Library/Application Support/Microsoft/Excel") // 남아있는 Excel 데이터
	license := mk("Library/Group Containers/UBF8T346G9.Office")
	mau := mk("Library/Application Support/Microsoft AutoUpdate")

	tgt := &Target{Kind: KindApp, BundleID: "com.microsoft.Word"}
	// Excel(다른 Office 앱)이 설치돼 있는 상황.
	augmentVendor(tgt, home, "com.microsoft.Word", "Microsoft Word",
		func() []string { return []string{"com.microsoft.Word", "com.microsoft.Excel"} })

	trashed := map[string]string{}
	for _, p := range tgt.TrashPaths {
		trashed[p.Path] = p.Confidence
	}
	if trashed[wordDir] != ConfLow {
		t.Errorf("Word 하위 폴더가 low로 안 잡힘: %v", tgt.TrashPaths)
	}
	if _, ok := trashed[license]; ok {
		t.Errorf("공유 라이선스(Group Containers)가 삭제 후보에 들어감 — 활성화 깨짐 위험: %s", license)
	}
	if _, ok := trashed[mau]; ok {
		t.Errorf("공유 AutoUpdate가 삭제 후보에 들어감: %s", mau)
	}
	excluded := map[string]bool{}
	for _, e := range tgt.Excluded {
		excluded[e.Path] = true
	}
	if !excluded[license] || !excluded[mau] {
		t.Errorf("공유 컴포넌트가 제외 목록에 없음. excluded=%v", tgt.Excluded)
	}
}

// Chrome이 유일한 구글 앱이면 Google 폴더 전체를 medium으로.
func TestAugmentVendorChromeSoleApp(t *testing.T) {
	home := t.TempDir()
	googleDir := filepath.Join(home, "Library/Application Support/Google")
	if err := os.MkdirAll(filepath.Join(googleDir, "Chrome"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 폴더 밖 공유 컴포넌트(Keystone LaunchAgent)도 고아이므로 정리 대상이어야 함.
	keystone := filepath.Join(home, "Library/LaunchAgents/com.google.keystone.agent.plist")
	if err := os.MkdirAll(filepath.Dir(keystone), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keystone, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tgt := &Target{Kind: KindApp, BundleID: "com.google.Chrome"}
	augmentVendor(tgt, home, "com.google.Chrome", "Google Chrome",
		func() []string { return []string{"com.google.Chrome"} }) // 구글 앱은 이것뿐

	var gotWhole, gotKeystone bool
	for _, p := range tgt.TrashPaths {
		if p.Path == googleDir {
			gotWhole = true
			if p.Confidence != ConfMedium {
				t.Errorf("Google 폴더 전체 확신도 = %s, want medium", p.Confidence)
			}
		}
		if p.Path == keystone {
			gotKeystone = true
			if p.Confidence != ConfMedium {
				t.Errorf("Keystone 확신도 = %s, want medium", p.Confidence)
			}
		}
	}
	if !gotWhole {
		t.Errorf("Google 폴더 전체가 medium 후보에 없음. paths=%v", tgt.TrashPaths)
	}
	if !gotKeystone {
		t.Errorf("고아 Keystone이 정리 대상에 없음. paths=%v", tgt.TrashPaths)
	}
	// 이 벤더 앱이 이것뿐이므로 제외 목록은 비어야 함.
	if len(tgt.Excluded) != 0 {
		t.Errorf("sole-app인데 제외 목록이 있음: %v", tgt.Excluded)
	}
}
