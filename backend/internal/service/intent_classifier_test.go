package service

import "testing"

func TestIntent_Greeting(t *testing.T) {
	c := NewIntentClassifier()
	cases := []string{
		"hi", "Hi", "HI", "hi!", "hello.", "hello,", "  hello  ",
		"hello there", "hi cfo", "hey", "hey team",
		"good morning", "good evening",
		"thanks", "thank you", "thx", "ty",
		"ok", "okay", "got it",
		"bye", "goodbye", "see you",
		"namaste", "hola",
	}
	for _, q := range cases {
		got := c.Classify(q)
		if got.Primary != IntentGreeting {
			t.Errorf("%q -> %s (kw=%v), want greeting", q, got.Primary, got.Keywords)
		}
		if !got.IsGreeting() {
			t.Errorf("%q -> IsGreeting=false, want true", q)
		}
	}
}

func TestIntent_GreetingDoesNotHijackRealQuestion(t *testing.T) {
	c := NewIntentClassifier()
	cases := []struct {
		q        string
		notWant  Intent
		mustWork bool
	}{
		{"What is the highest revenue we ever had?", IntentGreeting, true},
		{"thanks for the data, show me last quarter's profit", IntentGreeting, true},
		{"hi, can you tell me what our cash balance is?", IntentGreeting, true},
		{"hello, what was profit?", IntentGreeting, true},
	}
	for _, tc := range cases {
		got := c.Classify(tc.q)
		if got.Primary == tc.notWant {
			t.Errorf("%q -> %s, must NOT be greeting (real question with substantive content)", tc.q, got.Primary)
		}
	}
}

func TestIntent_Lookup(t *testing.T) {
	c := NewIntentClassifier()
	cases := []string{
		"What was our profit last quarter?",
		"Show me the current cash balance",
		"How much revenue did we generate in Q1?",
	}
	for _, q := range cases {
		got := c.Classify(q)
		if got.Primary != IntentLookup {
			t.Errorf("%q -> primary=%s, want lookup", q, got.Primary)
		}
	}
}

func TestIntent_Explain(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("Why did our margins drop in Q2?")
	if got.Primary != IntentExplain {
		t.Errorf("got %s, want explain", got.Primary)
	}
}

func TestIntent_Compare(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("Compare Q1 vs Q2 revenue")
	if got.Primary != IntentCompare {
		t.Errorf("got %s, want compare; keywords=%v", got.Primary, got.Keywords)
	}
}

func TestIntent_Trend(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("Show me the revenue trend over the last year")
	if got.Primary != IntentTrend {
		t.Errorf("got %s, want trend", got.Primary)
	}
}

func TestIntent_Forecast(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("What will our runway be next quarter?")
	if got.Primary != IntentForecast {
		t.Errorf("got %s, want forecast", got.Primary)
	}
}

func TestIntent_OutOfScope(t *testing.T) {
	c := NewIntentClassifier()
	cases := []string{
		"What's the weather like today?",
		"Tell me a joke",
		"Write me a Python script to sort a list",
		"Who won the cricket match yesterday?",
		"Are you ChatGPT?",
	}
	for _, q := range cases {
		got := c.Classify(q)
		if got.Primary != IntentOutOfScope {
			t.Errorf("%q -> %s, want out_of_scope", q, got.Primary)
		}
		if !got.IsRefusable() {
			t.Errorf("%q -> IsRefusable=false, want true", q)
		}
	}
}

func TestIntent_Unknown(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("xyzzy random nonsense input")
	if got.Primary != IntentUnknown {
		t.Errorf("got %s, want unknown", got.Primary)
	}
}

func TestIntent_Empty(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("")
	if got.Primary != IntentUnknown {
		t.Errorf("empty input should be unknown, got %s", got.Primary)
	}
}

func TestIntent_ScoreNormalized(t *testing.T) {
	c := NewIntentClassifier()
	got := c.Classify("What was our revenue, what is our cash, show me the latest")
	if got.Score < 0 || got.Score > 1 {
		t.Errorf("score should be in [0,1], got %f", got.Score)
	}
}

func TestIntent_Determinism(t *testing.T) {
	c := NewIntentClassifier()
	q := "What is our cash position and what was profit?"
	a := c.Classify(q)
	b := c.Classify(q)
	if a.Primary != b.Primary || a.Score != b.Score {
		t.Errorf("classifier non-deterministic: a=%+v b=%+v", a, b)
	}
}
