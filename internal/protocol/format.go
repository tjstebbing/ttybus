package protocol

import "strings"

func format(verb string, rest ...string) string {
	if len(rest) == 0 {
		return verb
	}
	var b strings.Builder
	b.WriteString(verb)
	for _, r := range rest {
		if r == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(r)
	}
	return b.String()
}

func FormatHello(h Hello) string {
	return format(VerbHello, helloKVs(h)...)
}

func FormatOK(kvs ...string) string {
	return format(VerbOK, kvs...)
}

func FormatErr(code, msg string) string {
	if msg == "" {
		return format(VerbErr, code)
	}
	return format(VerbErr, code, msg)
}

func FormatSub(channel string) string   { return format(VerbSub, channel) }
func FormatUnsub(channel string) string { return format(VerbUnsub, channel) }

func FormatPub(channel, payload string) string {
	if payload == "" {
		return format(VerbPub, channel)
	}
	return VerbPub + " " + channel + " " + payload
}

func FormatMsg(channel, payload string) string {
	if payload == "" {
		return format(VerbMsg, channel)
	}
	return VerbMsg + " " + channel + " " + payload
}

func FormatSend(id, channel, payload string) string {
	if payload == "" {
		return format(VerbSend, id, channel)
	}
	return VerbSend + " " + id + " " + channel + " " + payload
}

func FormatList() string { return VerbList }

func FormatPeer(h Hello) string {
	return format(VerbPeer, helloKVs(h)...)
}

func helloKVs(h Hello) []string {
	var kvs []string
	if h.ID != "" {
		kvs = append(kvs, "id="+h.ID)
	}
	if h.Name != "" {
		kvs = append(kvs, "name="+h.Name)
	}
	if h.PID != "" {
		kvs = append(kvs, "pid="+h.PID)
	}
	if h.Pane != "" {
		kvs = append(kvs, "pane="+h.Pane)
	}
	if h.CWD != "" {
		kvs = append(kvs, "cwd="+h.CWD)
	}
	if len(h.Subs) > 0 {
		kvs = append(kvs, "subs="+strings.Join(h.Subs, ","))
	}
	return kvs
}
