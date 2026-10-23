package rumable

import (
	"encoding/json"
	"errors"
)

// Old snapshots never owned dynamic compression. Ignore only those new fields
// when comparing against an old snapshot; all other ownership checks stay exact.
func sameIISSnapshot(live, expected json.RawMessage) bool {
	var a, b map[string]json.RawMessage
	if json.Unmarshal(live, &a) != nil || json.Unmarshal(expected, &b) != nil {
		return false
	}
	if _, ok := b["dynamicCompressionPresent"]; !ok {
		delete(a, "dynamicCompressionPresent")
		delete(a, "dynamicCompressionValue")
	}
	if _, ok := b["rumRuntime"]; !ok {
		delete(a, "rumRuntime")
	}
	if _, ok := b["cacheGuard"]; !ok {
		var guard struct {
			Rules []string `json:"rules"`
		}
		if raw, exists := a["cacheGuard"]; exists && (json.Unmarshal(raw, &guard) != nil || len(guard.Rules) != 0) {
			return false
		}
		delete(a, "cacheGuard")
	}
	x, _ := json.Marshal(a)
	return sameJSON(x, expected)
}

func dynamicCompressionSnapshot(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields
}

func copyDynamicCompression(raw, source json.RawMessage) (json.RawMessage, error) {
	fields := dynamicCompressionSnapshot(raw)
	baseline := dynamicCompressionSnapshot(source)
	if fields == nil {
		return raw, nil
	}
	if baseline["dynamicCompressionPresent"] != nil && fields["dynamicCompressionPresent"] == nil {
		fields["dynamicCompressionPresent"] = baseline["dynamicCompressionPresent"]
		fields["dynamicCompressionValue"] = baseline["dynamicCompressionValue"]
	}
	if baseline["cacheGuard"] != nil && fields["cacheGuard"] == nil {
		fields["cacheGuard"] = baseline["cacheGuard"]
	}
	if baseline["rumRuntime"] != nil && fields["rumRuntime"] == nil {
		fields["rumRuntime"] = baseline["rumRuntime"]
	}
	return json.Marshal(fields)
}

// Adopt the administrator's current dynamic setting and request-header permissions only on the first mutation
// under the new CLI. The transaction journal persists this baseline BEFORE IIS
// is changed, so failure recovery and disable can restore inheritance exactly.
func adoptIISDynamicCompression(root string, st *State) error {
	if st.Current.Site.IIS == nil || (dynamicCompressionSnapshot(st.OriginalIIS)["dynamicCompressionPresent"] != nil && dynamicCompressionSnapshot(st.OriginalIIS)["cacheGuard"] != nil && dynamicCompressionSnapshot(st.OriginalIIS)["rumRuntime"] != nil) {
		return nil
	}
	live, err := nativeCapture(root, st.Current.Site)
	if err != nil {
		return err
	}
	if !sameIISSnapshot(live, st.Current.Native) {
		return errors.New("IIS state changed before adopting RUM compression/cache prerequisites")
	}
	if st.Current.Native, err = copyDynamicCompression(st.Current.Native, live); err != nil {
		return err
	}
	if st.OriginalIIS, err = copyDynamicCompression(st.OriginalIIS, live); err != nil {
		return err
	}
	if st.Previous != nil {
		previous := *st.Previous
		if previous.Native, err = copyDynamicCompression(previous.Native, live); err != nil {
			return err
		}
		st.Previous = &previous
	}
	return nil
}

// Compare only owned RUM configuration, not OTel environment or metadata added
// while migrating an old snapshot. Recovery compares the failed deployment to
// the restored snapshot, even if a prior attempt already wrote that snapshot.
func iisRUMChanged(before, after json.RawMessage) bool {
	a, b := dynamicCompressionSnapshot(before), dynamicCompressionSnapshot(after)
	for _, key := range []string{"rules", "preconditions", "virtualDirectory", "compressionPresent", "compressionValue", "dynamicCompressionPresent", "dynamicCompressionValue", "cacheGuard"} {
		if !sameJSON(a[key], b[key]) {
			return true
		}
	}
	return false
}

func iisDynamicCompressionDisabled(raw json.RawMessage) bool {
	fields := dynamicCompressionSnapshot(raw)
	return string(fields["dynamicCompressionPresent"]) == "true" && string(fields["dynamicCompressionValue"]) == `"false"`
}

// A deliberate history rollback restores the old SDK/configuration while
// retaining the new RUM compression prerequisite. Transaction recovery instead
// restores the exact before snapshot, including the old dynamic setting.
func protectIISRollback(root string, n *Snapshot) error {
	if n.Site.IIS == nil || !n.Enabled {
		return nil
	}
	fields := dynamicCompressionSnapshot(n.Native)
	if fields == nil || fields["dynamicCompressionPresent"] == nil {
		return nil
	}
	fields["dynamicCompressionPresent"] = json.RawMessage("true")
	fields["dynamicCompressionValue"] = json.RawMessage(`"false"`)
	var err error
	n.Native, err = json.Marshal(fields)
	if err == nil && fields["cacheGuard"] != nil {
		n.Native, err = iisCommand(root, n.Site, "prepare-rum-rollback", map[string]any{"snapshot": n.Native, "script": n.Script, "injection": injectionTags(*n)})
	}
	return err
}

func checkIISRumCompression(root string, s Site) error {
	// RUM's compression consent must not revalidate live OTel credentials when
	// restoring a historical deployment that already has saved credentials.
	if s.OTel != nil {
		otel := *s.OTel
		otel.Enabled = false
		s.OTel = &otel
	}
	return nativeCheck(root, s, true)
}

func iisCacheGuardEnabled(raw json.RawMessage) bool {
	var guard struct {
		Rules []string `json:"rules"`
	}
	return json.Unmarshal(dynamicCompressionSnapshot(raw)["cacheGuard"], &guard) == nil && len(guard.Rules) == 1
}
