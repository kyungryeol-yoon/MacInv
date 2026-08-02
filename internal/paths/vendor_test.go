package paths

import "testing"

func TestVendorsEmbedLoads(t *testing.T) {
	vs := Vendors()
	if len(vs) == 0 {
		t.Fatal("임베드된 vendors.json이 비었음")
	}
	if _, _, ok := LookupVendor(vs, "com.google.Chrome", "Google Chrome"); !ok {
		t.Error("Google 벤더를 찾지 못함")
	}
}

func TestLookupVendor(t *testing.T) {
	vs := Vendors()

	v, sub, ok := LookupVendor(vs, "com.google.Chrome", "Google Chrome")
	if !ok || v.Name != "Google" || sub != "Chrome" {
		t.Errorf("Chrome 매핑 오류: v=%s sub=%q ok=%v", v.Name, sub, ok)
	}
	// apps 맵에 없는 구글 앱은 이름에서 벤더명을 떼어 추정.
	_, sub2, ok2 := LookupVendor(vs, "com.google.SomethingNew", "Google Something")
	if !ok2 || sub2 != "Something" {
		t.Errorf("추정 하위폴더 오류: sub=%q ok=%v", sub2, ok2)
	}
	// 매칭 안 되는 것.
	if _, _, ok3 := LookupVendor(vs, "org.godotengine.godot", "Godot"); ok3 {
		t.Error("Godot이 벤더에 매칭되면 안 됨")
	}
}

func TestCountVendorApps(t *testing.T) {
	v, _, _ := LookupVendor(Vendors(), "com.google.Chrome", "Google Chrome")
	installed := []string{"com.google.Chrome", "com.google.drivefs", "com.apple.Safari", "org.godotengine.godot"}
	if n := CountVendorApps(v, installed, "com.google.Chrome"); n != 1 {
		t.Errorf("다른 구글 앱 수 = %d, want 1 (drivefs)", n)
	}
	if n := CountVendorApps(v, []string{"com.google.Chrome"}, "com.google.Chrome"); n != 0 {
		t.Errorf("이 앱뿐인데 = %d, want 0", n)
	}
}
