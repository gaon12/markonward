# Markonward

[English](README.md)

Markonward는 Markdown을 공개된 소스 매핑 AST로 파싱하고, 그 AST를 안전한
HTML·구조화된 일반 텍스트·정규화 Markdown으로 렌더링하는 Go 도구 모음입니다.
네 번째 프로필 EnhanceMark는 CommonMark와 GFM의 동작을 바꾸지 않으면서
한국어 문장에서 사용자의 의도를 보수적으로 복구합니다.

> **개발 상태:** 현재 저장소는 v1 이전 구현 스냅샷이며 v1.0 적합 릴리스가
> 아닙니다. 고정된 공식 테스트에서 CommonMark 0.31.2는 357/652, GFM 0.29는
> 351/649 예제를 통과합니다. 릴리스 워크플로는 100%를 요구하므로 현재
> `v1.0.0`을 배포할 수 없습니다.

## Markonward를 만드는 이유

- 파서, 공개 AST, 각 렌더러를 서로 독립적으로 가져다 쓸 수 있습니다.
- 런타임 코드는 Go 표준 라이브러리만 사용합니다.
- 저할당 경로인 `Parse`는 입력을 빌리고, `ParseCopy`와 `ParseReader`는 입력을
  소유합니다.
- 추적을 끄면 trace 이벤트를 만들지 않습니다. 켜면 안정된 순서의 결정을
  JSON Lines 또는 한국어·영어 텍스트로 출력합니다.
- EnhanceMark는 `서울~부산`, `9시~18시`, `1~3명` 같은 한국어 범위와 의도한
  단일 물결표 취소선을 구분합니다.
- Parser와 Renderer 설정은 불변이며 여러 goroutine에서 함께 쓸 수 있습니다.

## 요구 사항과 로컬 실행

Go 1.26 이상이 필요합니다.

```sh
go test ./...
go run ./cmd/markonward convert README.ko.md --from enhance --to html
go run ./cmd/markonward explain README.ko.md --profile enhance --locale ko
```

모듈 경로는 `github.com/gaon12/markonward`입니다. 릴리스 게이트를 모두
통과하기 전에는 안정 태그를 가정하지 말고 특정 커밋을 사용하세요.

## 라이브러리 API

라이브러리는 프로필을 반드시 명시해야 합니다.

```go
p, err := parser.New(
    profile.EnhanceMarkV1,
    parser.WithTrace(sink),
    parser.WithRecovery(ast.Strong, parser.RecoverAtParagraphEnd),
)
if err != nil {
    return err
}

result, err := p.Parse(ctx, source) // source를 빌림
if err != nil {
    return err
}

if err := html.New().Render(ctx, dst, result.Document); err != nil {
    return err
}
if err := plaintext.New().Render(ctx, dst, result.Document); err != nil {
    return err
}
return markdown.New(profile.EnhanceMarkV1).Render(ctx, dst, result.Document)
```

렌더러만 쓰는 개발자는 `ast.NewBuilder`로 문서를 만들 수 있습니다. 파서만
쓰면 renderer 패키지는 의존 그래프에 들어오지 않습니다.

`ast.Span`은 0부터 시작하는 UTF-8 byte half-open 범위입니다.
`Document.Position`은 1부터 시작하는 줄과 Unicode code-point 열을 필요할 때
계산합니다.

## 프로필

| 프로필 | 기반 | GFM 확장 | Enhance 추론 |
| --- | --- | --- | --- |
| `CommonMark0312` | CommonMark 0.31.2 | 없음 | 절대 적용 안 함 |
| `GFM029` | CommonMark 0.29 | 표, 작업 목록, 취소선, 자동 링크, tagfilter | 절대 적용 안 함 |
| `GFM` | CommonMark 0.31.2 | 표, 작업 목록, 취소선, 자동 링크, tagfilter | 절대 적용 안 함 |
| `EnhanceMarkV1` | 현대형 GFM | 현대형 GFM 전체 | 한국어 범위, 짝 구두점, 문단 끝 복구 |

프로필 이름은 목표 계약을 나타냅니다. 공식 예제가 전부 통과하기 전까지는
이 문서 위쪽의 적합성 수치가 실제 구현 상태의 기준입니다.

## CLI

```text
markonward convert [FILE] --from enhance|commonmark|gfm|gfm029 \
  --to html|text|markdown [-o FILE] [--unsafe-html]

markonward explain [FILE] --profile enhance|commonmark|gfm|gfm029 \
  --format text|jsonl --locale en|ko --level decisions|verbose
```

`FILE`을 생략하면 표준 입력, `-o`를 생략하면 표준 출력을 사용합니다. CLI의
기본 프로필은 EnhanceMark지만 라이브러리에는 기본 프로필이 없습니다. 올바른
UTF-8이 아닌 입력은 명시적으로 거부합니다.

## 문서

- [아키텍처와 소유권](docs/architecture.ko.md)
- [EnhanceMark v1 규칙](docs/enhancemark.ko.md)
- [추적 스키마와 rule ID](docs/trace.ko.md)
- [보안 모델](docs/security.ko.md)
- [벤치마크와 릴리스 기준](docs/benchmarks.ko.md)

각 문서에는 같은 범위의 영문 문서 링크가 있습니다.

## 검증

```sh
./scripts/check.sh
MARKONWARD_STRICT_SPECS=1 go test -run 'TestOfficial' ./parser
go test -C benchmarks -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' \
  -benchmem -count 10 ./... > benchmarks/results/current.txt
go run ./internal/benchgate -input benchmarks/results/current.txt
```

명세 fixture는 SHA-256으로 고정되어 있고 `testdata/spec`에 별도의 CC BY-SA
고지가 있습니다. 비교 대상 goldmark v1.8.4는 중첩된 `benchmarks` 모듈에만
존재하므로 런타임 의존성이 아닙니다.

## 라이선스

Markonward 소스 코드는 [MIT License](LICENSE)를 따릅니다. 업스트림 명세
fixture에는 [별도 고지](testdata/spec/NOTICE.md)가 적용됩니다.
