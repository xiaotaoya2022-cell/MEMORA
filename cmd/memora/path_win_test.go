package main

import "testing"

func TestUpdateUserPathString(t *testing.T) {
	t.Run("add to empty", func(t *testing.T) {
		got, changed := updateUserPathString("", `C:\Program Files\MEMORA\`, true)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		if got != `C:\Program Files\MEMORA` {
			t.Fatalf("got %q, want %q", got, `C:\Program Files\MEMORA`)
		}
	})

	t.Run("deduplicate case-insensitive with trailing slash", func(t *testing.T) {
		cur := `%USERPROFILE%\AppData\Local\Programs;c:\program files\memora\;D:\Tools`
		got, changed := updateUserPathString(cur, `C:\Program Files\MEMORA`, true)
		want := `%USERPROFILE%\AppData\Local\Programs;D:\Tools;C:\Program Files\MEMORA`
		if got != want {
			t.Fatalf("got %q, want %q (changed=%v)", got, want, changed)
		}
		// Running add again on canonical result should report changed=false.
		got2, changed2 := updateUserPathString(got, `C:\Program Files\MEMORA`, true)
		if changed2 || got2 != want {
			t.Fatalf("second add should be no-op: got %q changed=%v", got2, changed2)
		}
	})

	t.Run("remove existing entry cleanly", func(t *testing.T) {
		cur := `C:\Windows\System32;C:\Program Files\MEMORA\;D:\Go\bin`
		got, changed := updateUserPathString(cur, `c:\program files\memora`, false)
		want := `C:\Windows\System32;D:\Go\bin`
		if !changed || got != want {
			t.Fatalf("got %q changed=%v, want %q changed=true", got, changed, want)
		}
	})

	t.Run("remove absent entry is no-op", func(t *testing.T) {
		cur := `C:\Windows\System32;D:\Go\bin`
		got, changed := updateUserPathString(cur, `C:\Program Files\MEMORA`, false)
		if changed || got != cur {
			t.Fatalf("got %q changed=%v, want %q changed=false", got, changed, cur)
		}
	})
}
