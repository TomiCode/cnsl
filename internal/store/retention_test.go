package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneBefore(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d := Device{MAC: "AA", APIKey: "k", FriendlyID: "F", Status: DevicePending, RefreshRate: 900}
	if err := st.CreateDevice(ctx, &d); err != nil {
		t.Fatal(err)
	}
	old := Time{time.Now().Add(-40 * 24 * time.Hour).UTC()}
	if err := st.InsertDeviceLogs(ctx, []DeviceLog{
		{DeviceID: d.ID, Message: "old", ReceivedAt: old},
		{DeviceID: d.ID, Message: "new", ReceivedAt: Now()},
	}); err != nil {
		t.Fatal(err)
	}
	n, err := st.pruneBefore(ctx, "device_logs", "received_at", time.Now().Add(-30*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v; want 1", n, err)
	}
	left, _ := st.DeviceLogs(ctx, LogFilter{DeviceID: d.ID, Limit: 10})
	if len(left) != 1 || left[0].Message != "new" {
		t.Fatalf("left = %+v", left)
	}
}
