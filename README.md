# gsb-rrule

Go 标准库实现的日历重复规则、时区和资源占用库。只依赖 `time` 等标准库，
`go test ./...` 即可验证；没有 HTTP 服务，也不是命令行工具。

## RRULE 子集

- `FREQ=DAILY|WEEKLY|MONTHLY|YEARLY`
- `INTERVAL`
- `BYDAY=MO,TU,WE,TH,FR,SA,SU`，可带序号（`2MO`、`-1FR`）
- `BYMONTHDAY`，支持负数（`-1` = 月末）和多个值；不存在的日期（如
  平年 2 月 29 日、小月 31 日）直接跳过
- `BYSETPOS`，支持负数（`-1` = 当期候选集最后一个）
- `COUNT` 与 `UNTIL` 二选一，也可以都不写
- `WKST` 默认周一，可改

`COUNT`/`UNTIL` 都没有时，展开必须给出闭区间时间窗，循环按时间窗和最大
周期数双重兜底，不会死循环。

## 三种时间模式

- TZID：`DTSTART` 带 `time.LoadLocation(...)` 得到的真实时区，例如
  `America/New_York`
- UTC：`DTSTART` 带 `time.UTC`
- floating：`DTStart.Location() == rrule.Floating`，只按墙上时钟展开

春令时拨快时，落在缺口里的「凌晨两点那场」不存在，直接跳过，不会被
Go 的规范化逻辑挪到三点。秋令时拨慢时，墙上时间存在两次：默认取较早
的一次（夏令时偏移），规则只展开一次；把 `Rule.Ambiguous` 设成
`SecondAmbiguous` 可取较晚的一次。floating 规则不绑时区，墙上 02:30 永远
只有一次。

`EXDATE` 抠掉某次，`RDATE` 加某次；绝对时间按瞬间匹配，floating 按墙上
时钟匹配。

## 用法

```go
ny, _ := time.LoadLocation("America/New_York")
r, _ := rrule.ParseRule("FREQ=WEEKLY;BYDAY=MO,WE;COUNT=10", ny)
cal := &rrule.Calendar{
    DTStart: time.Date(2024, 3, 4, 9, 0, 0, 0, ny),
    Rule:    r,
    ExDates: []time.Time{time.Date(2024, 3, 11, 9, 0, 0, 0, ny)},
}
occ, _ := cal.Between(
    time.Date(2024, 3, 1, 0, 0, 0, 0, ny),
    time.Date(2024, 6, 1, 0, 0, 0, 0, ny),
)
```

floating 的「每天九点」：

```go
start := time.Date(2024, 3, 4, 9, 0, 0, 0, rrule.Floating)
cal := &rrule.Calendar{DTStart: start, Rule: mustParse("FREQ=DAILY")}
from := time.Date(2024, 3, 4, 0, 0, 0, 0, rrule.Floating)
to   := time.Date(2024, 3, 8, 23, 59, 0, 0, rrule.Floating)
```

## 资源占用

`Event` 有全天和带时刻两种；所有事件最终转成同一套半开绝对区间
`[Start, End)`：

- 一个会在另一个开始的同一刻结束（如 10:00 和 10:00）不算冲突，
  背靠背允许
- 全天事件是日期段，跨日全天事件覆盖 `Start` 到 `End` 的每一天，
  右端为次日 00:00
- 跨时区、UTC、floating 全部先转绝对瞬间再比较；floating 需要一个
  observer 时区来锚定（同一个 floating 09:00 在纽约和在伦敦是不同的瞬间）

```go
a := rrule.NewTimedEvent("room-a", startA, endA)
b := rrule.NewTimedEvent("room-a", startB, endB)
if rrule.Overlaps(a, b, ny) { ... }

day := rrule.NewAllDayEvent("room-a", midnight, endMidnightOptional)
hits := rrule.Conflicts(day, existing, ny)
```

## 测试

`go test ./...`

覆盖：INTERVAL/WKST、月末 31 号（平/闰 2 月、小月）、第二个周一、
倒数一个周五、负数 BYSETPOS、COUNT/UNTIL 互斥校验、纽约/伦敦春拨快与
秋拨慢、floating、EXDATE/RDATE、闭区间窗口、全天与带时刻/跨日/跨时区
冲突及半开边界。
