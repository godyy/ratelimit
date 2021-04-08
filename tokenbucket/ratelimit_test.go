package tokenbucket

import (
	gc "gopkg.in/check.v1"
	"math"
	"testing"
	"time"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type rateLimitSuite struct{}

var _ = gc.Suite(rateLimitSuite{})

type takeReq struct {
	time       time.Duration
	count      int64
	expectWait time.Duration
}

var takeTests = []struct {
	about        string
	fillInterval time.Duration
	capacity     int64
	reqs         []takeReq
}{{
	about:        "serial requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      0,
		expectWait: 0,
	}, {
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       0,
		count:      1,
		expectWait: 250 * time.Millisecond,
	}, {
		time:       250 * time.Millisecond,
		count:      1,
		expectWait: 250 * time.Millisecond,
	}},
}, {
	about:        "concurrent requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       0,
		count:      2,
		expectWait: 500 * time.Millisecond,
	}, {
		time:       0,
		count:      2,
		expectWait: 1000 * time.Millisecond,
	}, {
		time:       0,
		count:      1,
		expectWait: 1250 * time.Millisecond,
	}},
}, {
	about:        "more than capacity",
	fillInterval: 1 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       20 * time.Millisecond,
		count:      15,
		expectWait: 5 * time.Millisecond,
	}},
}, {
	about:        "sub-quantum time",
	fillInterval: 10 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       7 * time.Millisecond,
		count:      1,
		expectWait: 3 * time.Millisecond,
	}, {
		time:       8 * time.Millisecond,
		count:      1,
		expectWait: 12 * time.Millisecond,
	}},
}, {
	about:        "within capacity",
	fillInterval: 10 * time.Millisecond,
	capacity:     5,
	reqs: []takeReq{{
		time:       0,
		count:      5,
		expectWait: 0,
	}, {
		time:       60 * time.Millisecond,
		count:      5,
		expectWait: 0,
	}, {
		time:       60 * time.Millisecond,
		count:      1,
		expectWait: 10 * time.Millisecond,
	}, {
		time:       80 * time.Millisecond,
		count:      2,
		expectWait: 10 * time.Millisecond,
	}},
}}

var availTests = []struct {
	about        string
	capacity     int64
	fillInterval time.Duration
	take         int64
	sleep        time.Duration

	expectCountAfterTake  int64
	expectCountAfterSleep int64
}{{
	about:                 "should fill tokens after interval",
	capacity:              5,
	fillInterval:          time.Second,
	take:                  5,
	sleep:                 time.Second,
	expectCountAfterTake:  0,
	expectCountAfterSleep: 1,
}, {
	about:                 "should fill tokens plus existing count",
	capacity:              2,
	fillInterval:          time.Second,
	take:                  1,
	sleep:                 time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 2,
}, {
	about:                 "shouldn't fill before interval",
	capacity:              2,
	fillInterval:          2 * time.Second,
	take:                  1,
	sleep:                 time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 1,
}, {
	about:                 "should fill only once after 1*interval before 2*interval",
	capacity:              2,
	fillInterval:          2 * time.Second,
	take:                  1,
	sleep:                 3 * time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 2,
}}

func (t rateLimitSuite) TestTake(c *gc.C) {
	for i, test := range takeTests {
		l := NewLimiter(test.fillInterval, test.capacity)
		for j, req := range test.reqs {
			d, ok := l.take(l.startTime.Add(req.time), req.count, infinityDuration)
			c.Assert(ok, gc.Equals, true)
			if d != req.expectWait {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expectWait)
			}
		}
	}
}

func (t rateLimitSuite) TestTakeMaxDuration(c *gc.C) {
	for i, test := range takeTests {
		l := NewLimiter(test.fillInterval, test.capacity)
		for j, req := range test.reqs {
			if req.expectWait > 0 {
				d, ok := l.take(l.startTime.Add(req.time), req.count, req.expectWait-1)
				c.Assert(ok, gc.Equals, false)
				c.Assert(d, gc.Equals, time.Duration(0))
			}
			d, ok := l.take(l.startTime.Add(req.time), req.count, req.expectWait)
			c.Assert(ok, gc.Equals, true)
			if d != req.expectWait {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expectWait)
			}
		}
	}
}

type takeAvailableReq struct {
	time   time.Duration
	count  int64
	expect int64
}

var takeAvailableTests = []struct {
	about        string
	fillInterval time.Duration
	capacity     int64
	reqs         []takeAvailableReq
}{{
	about:        "serial requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  0,
		expect: 0,
	}, {
		time:   0,
		count:  10,
		expect: 10,
	}, {
		time:   0,
		count:  1,
		expect: 0,
	}, {
		time:   250 * time.Millisecond,
		count:  1,
		expect: 1,
	}},
}, {
	about:        "concurrent requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  5,
		expect: 5,
	}, {
		time:   0,
		count:  2,
		expect: 2,
	}, {
		time:   0,
		count:  5,
		expect: 3,
	}, {
		time:   0,
		count:  1,
		expect: 0,
	}},
}, {
	about:        "more than capacity",
	fillInterval: 1 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  10,
		expect: 10,
	}, {
		time:   20 * time.Millisecond,
		count:  15,
		expect: 10,
	}},
}, {
	about:        "within capacity",
	fillInterval: 10 * time.Millisecond,
	capacity:     5,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  5,
		expect: 5,
	}, {
		time:   60 * time.Millisecond,
		count:  5,
		expect: 5,
	}, {
		time:   70 * time.Millisecond,
		count:  1,
		expect: 1,
	}},
}}

func (rateLimitSuite) TestTakeAvailable(c *gc.C) {
	for i, test := range takeAvailableTests {
		l := NewLimiter(test.fillInterval, test.capacity)
		for j, req := range test.reqs {
			d := l.takeAvailable(l.startTime.Add(req.time), req.count)
			if d != req.expect {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expect)
			}
		}
	}
}

func (rateLimitSuite) TestPanics(c *gc.C) {
	c.Assert(func() { NewLimiter(0, 1) }, gc.PanicMatches, "token bucket fill interval is not > 0")
	c.Assert(func() { NewLimiter(-2, 1) }, gc.PanicMatches, "token bucket fill interval is not > 0")
	c.Assert(func() { NewLimiter(1, 0) }, gc.PanicMatches, "token bucket capacity is not > 0")
	c.Assert(func() { NewLimiter(1, -2) }, gc.PanicMatches, "token bucket capacity is not > 0")
}

func isCloseTo(x, y, tolerance float64) bool {
	return math.Abs(x-y)/y < tolerance
}

func (rateLimitSuite) TestRate(c *gc.C) {
	l := NewLimiter(1, 1)
	if !isCloseTo(l.Rate(), 1e9, 0.00001) {
		c.Fatalf("got %v want 1e9", l.Rate())
	}

	l = NewLimiter(2*time.Second, 1)
	if !isCloseTo(l.Rate(), 0.5, 0.00001) {
		c.Fatalf("got %v want 0.5", l.Rate())
	}

	l = NewLimiterWithQuantum(100*time.Millisecond, 5, 1)
	if !isCloseTo(l.Rate(), 50, 0.00001) {
		c.Fatalf("got %v want 50", l.Rate())
	}
}

func checkRate(c *gc.C, rate float64) {
	l := NewLimiterWithRate(rate, 1<<62)
	if !isCloseTo(l.Rate(), rate, rateMargin) {
		c.Fatalf("got %g want %v", l.Rate(), rate)
	}

	d, ok := l.take(l.startTime, 1<<62, infinityDuration)
	c.Assert(ok, gc.Equals, true)
	c.Assert(d, gc.Equals, time.Duration(0))

	// Check that the actual rate is as expected by
	// asking for a not-quite multiple of the bucket's
	// quantum and checking that the wait time
	// correct.
	d, ok = l.take(l.startTime, l.quantum*2-l.quantum/2, infinityDuration)
	c.Assert(ok, gc.Equals, true)
	expectTime := 1e9 * float64(l.quantum) * 2 / rate
	if !isCloseTo(float64(d), expectTime, rateMargin) {
		c.Fatalf("rate %g: got %g want %v", rate, float64(d), expectTime)
	}
}

func (rateLimitSuite) NewLimiterWithRate(c *gc.C) {
	for rate := float64(1); rate < 1e6; rate += 7 {
		checkRate(c, rate)
	}
	for _, rate := range []float64{
		1024 * 1024 * 1024,
		1e-5,
		0.9e-5,
		0.5,
		0.9,
		0.9e8,
		3e12,
		4e18,
		float64(1<<63 - 1),
	} {
		checkRate(c, rate)
		checkRate(c, rate/3)
		checkRate(c, rate*1.3)
	}
}

func TestAvailable(t *testing.T) {
	for i, tt := range availTests {
		l := NewLimiter(tt.fillInterval, tt.capacity)
		if c := l.takeAvailable(l.startTime, tt.take); c != tt.take {
			t.Fatalf("#%d: %s, take = %d, want = %d", i, tt.about, c, tt.take)
		}
		if c := l.available(l.startTime); c != tt.expectCountAfterTake {
			t.Fatalf("#%d: %s, after take, available = %d, want = %d", i, tt.about, c, tt.expectCountAfterTake)
		}
		if c := l.available(l.startTime.Add(tt.sleep)); c != tt.expectCountAfterSleep {
			t.Fatalf("#%d: %s, after some time it should fill in new tokens, available = %d, want = %d",
				i, tt.about, c, tt.expectCountAfterSleep)
		}
	}

}

func TestNoBonusTokenAfterBucketIsFull(t *testing.T) {
	tb := NewLimiterWithQuantum(time.Second*1, 20, 100)
	curAvail := tb.Available()
	if curAvail != 100 {
		t.Fatalf("initially: actual available = %d, expected = %d", curAvail, 100)
	}

	time.Sleep(time.Second * 5)

	curAvail = tb.Available()
	if curAvail != 100 {
		t.Fatalf("after pause: actual available = %d, expected = %d", curAvail, 100)
	}

	cnt := tb.TakeAvailable(100)
	if cnt != 100 {
		t.Fatalf("taking: actual taken count = %d, expected = %d", cnt, 100)
	}

	curAvail = tb.Available()
	if curAvail != 0 {
		t.Fatalf("after taken: actual available = %d, expected = %d", curAvail, 0)
	}
}

func BenchmarkWait(b *testing.B) {
	tb := NewLimiter(1, 16*1024)
	for i := b.N - 1; i >= 0; i-- {
		tb.Wait(1)
	}
}

func BenchmarkNewLimiter(b *testing.B) {
	for i := b.N - 1; i >= 0; i-- {
		NewLimiterWithRate(4e18, 1<<62)
	}
}