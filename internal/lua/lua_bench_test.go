package lua

import (
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// BenchmarkCoroutineCreation 测试协程创建开销
func BenchmarkCoroutineCreation(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coro, err := engine.NewCoroutine(nil)
		if err != nil {
			b.Fatal(err)
		}
		engine.releaseCoroutine(coro)
	}
}

// BenchmarkLuaContextPool 测试 LuaContext 池化开销
func BenchmarkLuaContextPool(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(engine, nil)
		ctx.SetVariable("key", "value")
		ctx.Write([]byte("hello"))
		ctx.SetPhase(PhaseContent)
		ctx.Release()
	}
}

// BenchmarkBytecodeCompilation 测试字节码编译开销
func BenchmarkBytecodeCompilation(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	script := `
		local x = 1 + 2
		local y = x * 3
		local z = "hello " .. "world"
		if y > 5 then
			return z
		end
		return x
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.CodeCache().GetOrCompileInline(script)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSharedDictSetGet 测试共享字典读写开销
func BenchmarkSharedDictSetGet(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	dict := engine.CreateSharedDict("bench", 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dict.Set("key", "value", 0)
		dict.Get("key")
	}
}

// BenchmarkTimerCallbackThroughput 测试定时器回调吞吐量
func BenchmarkTimerCallbackThroughput(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	manager := engine.TimerManager()
	callback := engine.L.NewFunction(func(L *glua.LState) int {
		return 0
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.At(1*time.Millisecond, callback, nil)
	}
	b.StopTimer()

	// 等待所有定时器完成
	manager.WaitAll(5 * time.Second)
}

// BenchmarkTimerCallbackWithLuaExecution 测试带 Lua 执行的定时器回调
func BenchmarkTimerCallbackWithLuaExecution(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, engine.TimerManager(), ngx)

	// 注册共享字典 API 供回调使用
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 简单的无 upvalue 回调
	script := `
		ngx.timer.at(0.001, function()
			-- empty callback
		end)
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := L.DoString(script); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	// 等待所有回调执行完成
	time.Sleep(100 * time.Millisecond)
}

// BenchmarkUpvalueDetection 测试 upvalue 检测开销
func BenchmarkUpvalueDetection(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, engine.TimerManager(), ngx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 尝试注册带有 upvalue 的回调（应该被拒绝）
		err := L.DoString(`
			local x = 42
			local ok, err = ngx.timer.at(1, function() return x end)
		`)
		if err != nil {
			// 错误可能被 Lua 内部处理，检查返回值
			_ = err
		}
	}
}

// BenchmarkTimerGracefulShutdown 测试优雅关闭开销
func BenchmarkTimerGracefulShutdown(b *testing.B) {
	for i := 0; i < b.N; i++ {
		engine, err := NewEngine(DefaultConfig())
		if err != nil {
			b.Fatal(err)
		}

		manager := engine.TimerManager()
		callback := engine.L.NewFunction(func(L *glua.LState) int {
			return 0
		})

		// 创建一些定时器
		for j := 0; j < 10; j++ {
			manager.At(1*time.Millisecond, callback, nil)
		}

		// 关闭引擎（包含优雅关闭）
		engine.Close()
	}
}

// BenchmarkLuaContextPoolReuse 测试 LuaContext 池复用率。
// 验证多次获取/释放后池能否有效复用对象，减少分配。
func BenchmarkLuaContextPoolReuse(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 模拟一次完整的请求生命周期
		ctx := NewContext(engine, nil)
		ctx.SetVariable("uri", "/test")
		ctx.SetVariable("method", "GET")
		ctx.SetPhase(PhaseContent)
		ctx.Write([]byte("response body"))
		ctx.FlushOutput()
		ctx.Release()
	}
}

// BenchmarkLuaCoroutinePoolThroughput 测试协程池吞吐量。
// 验证协程池在高频率创建/销毁场景下的复用效果。
func BenchmarkLuaCoroutinePoolThroughput(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			coro, err := engine.NewCoroutine(nil)
			if err != nil {
				continue
			}
			coro.Close()
		}
	})
}

// BenchmarkLuaTablePool 测试 Lua table 对象池性能。
// 验证 table 创建和池复用在频繁 table 操作场景下的表现。
func BenchmarkLuaTablePool(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	L := engine.L

	b.Run("NewTable_NoPool", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			t := L.NewTable()
			t.RawSetString("key1", glua.LString("value1"))
			t.RawSetString("key2", glua.LString("value2"))
			t.RawSetString("key3", glua.LNumber(42))
		}
	})

	b.Run("SharedDict_AsPool", func(b *testing.B) {
		// 共享字典底层使用对象池存储条目
		dict := engine.CreateSharedDict("bench_pool", 1000)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dict.Set("key", "value_with_pool", 0)
			dict.Get("key")
			dict.Delete("key")
		}
	})
}
