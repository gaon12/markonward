# 벤치마크와 릴리스 기준

[English](benchmarks.md)

비교 코드는 중첩 모듈에 있으므로 goldmark v1.8.4가 Markonward 런타임
의존성이 되지 않습니다. trace를 끈 현대형 GFM끼리 비교합니다.

## 코퍼스

| Fixture | 목적 |
| --- | --- |
| `small` | 짧은 heading, paragraph, emphasis, link |
| `korean` | 한국어 범위, 짝 구두점, 작업 목록 |
| `table` | 정렬된 GFM 표와 inline 서식 |
| `readme` | 문서 형태 section 반복 |
| `delimiters` | 중첩 및 적대적 delimiter 부하 |

각 fixture를 parser-only와 parse+HTML로 실행합니다. parse+HTML은 sanitizer
비용이 아니라 같은 변환 범위를 비교하기 위해 trusted HTML mode를 사용합니다.

## 재현

전원·온도 정책을 고정한 유휴 장비에서 실행하세요. gate는 10개 sample을
요구합니다.

```sh
mkdir -p benchmarks/results
sh ./scripts/benchmark.sh benchmarks/results/current.txt 10

go tool -C tools benchstat ../benchmarks/results/current.txt

go run ./internal/benchgate -input benchmarks/results/current.txt
```

PowerShell redirect는 UTF-16 파일을 만들 수 있습니다. `benchgate`는 UTF-8,
UTF-16LE, UTF-16BE benchmark 파일을 모두 읽습니다.

Windows에서는 `./scripts/benchmark.ps1`이 같은 결과를 만듭니다. 한 구현을 10회
연속 측정해 장기 host drift가 편향되지 않도록 각 표본을 독립적인
`go test -count 1`로 실행하고 두 구현을 인접 측정합니다.

## v1 릴리스 gate

`BenchmarkParse`와 `BenchmarkParseHTML` 각각에 다음을 적용합니다.

1. 구현별로 fixture와 metric의 기하평균을 구합니다.
2. 모든 fixture에서 Markonward의 `ns/op`, `B/op`, `allocs/op` 비율이
   goldmark의 `1.15x` 이하여야 합니다.
3. 다섯 fixture 비율의 기하평균을 구합니다.
4. 세 metric 모두 Markonward가 엄격히 `1.0x` 미만이어야 합니다.

`internal/benchgate`가 이 규칙을 구현하며 pair 누락 또는 10개 미만 sample을
거부합니다. GitHub 공유 runner는 회귀 artifact에는 유용하지만 시간 편차가
큽니다. 최종 릴리스 판단은 제어된 host에서도 재현해야 합니다.

## 현재 스냅샷

2026-07-21의 인접 측정 10회 로컬 실행은 release gate를 통과했습니다. parser
기하평균 비율은 `ns/op 0.811x`, `B/op 0.743x`, `allocs/op 0.304x`,
parse+HTML은 각각 `0.700x`, `0.520x`, `0.537x`였습니다. 가장 느린 시간 비율은
parser `korean`의 `1.031x`와 parse+HTML `delimiters`의 `1.058x`로, 모두
fixture별 `1.15x` 상한 안입니다. 오래된 2코어 Windows host는 여전히 편차가
크므로 이는 홍보 수치가 아니라 gate 증거입니다. release CI가 같은 인접 측정
방법을 다시 실행하며 어떤 v1 tag도 이를 우회할 수 없습니다.
