package sshd

import "testing"

func TestTheManagedBlockReplacesOnlyItself(t *testing.T) {
	mine := "Host work\n  HostName work.example\n"
	once := replaceBlock(mine, "Host desk\n  HostName desk.ts.net")
	want := mine + "\n" + blockBegin + "\nHost desk\n  HostName desk.ts.net\n" + blockEnd + "\n"
	if once != want {
		t.Fatalf("first write:\n%s\nwant:\n%s", once, want)
	}
	if again := replaceBlock(once, "Host desk\n  HostName desk.ts.net"); again != once {
		t.Fatalf("rewriting the same block changed the file:\n%s", again)
	}
	after := replaceBlock(once+"Host later\n", "Host laptop\n  HostName laptop.ts.net")
	if after != mine+"\n"+blockBegin+"\nHost laptop\n  HostName laptop.ts.net\n"+blockEnd+"\nHost later\n" {
		t.Fatalf("replacing the block lost what follows it:\n%s", after)
	}
	if gone := replaceBlock(once, ""); gone != mine+"\n" {
		t.Fatalf("an empty block should leave only your lines:\n%q", gone)
	}
}

func TestConfigEntriesTrustTailnetHostsOnFirstContact(t *testing.T) {
	got := configEntries([]Host{{Name: "desk", Address: "desk.ts.net", User: "ahmed"}, {Name: "lap", Address: "lap.ts.net"}})
	want := "Host desk\n  HostName desk.ts.net\n  User ahmed\n  StrictHostKeyChecking accept-new\n" +
		"Host lap\n  HostName lap.ts.net\n  StrictHostKeyChecking accept-new"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestParseHostsReadsBackWrittenEntries(t *testing.T) {
	hosts := []Host{{Name: "desk", Address: "desk.ts.net", User: "ahmed"}, {Name: "lap", Address: "lap.ts.net"}}
	text := replaceBlock("Host mine\n  User me\n", configEntries(hosts))
	got := parseHosts(text)
	if len(got) != 2 || got[0] != hosts[0] || got[1] != hosts[1] {
		t.Fatalf("got %+v", got)
	}
	if parseHosts("Host mine\n") != nil {
		t.Fatal("hosts outside spillup's block must be ignored")
	}
}
