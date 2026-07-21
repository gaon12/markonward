# 아키텍처와 소유권

[English](architecture.md)

## 패키지 경계

```text
profile ─┐
         ├─> parser ─> ast <─ renderer/html
trace ───┤              ├─── renderer/plaintext
diagnostic┘              └─── renderer/markdown

extension ─> parser syntax/transform pipeline + custom render handler
markonward ─> 선택적인 parser + renderer 조합
cmd/markonward ─> CLI 전용
```

코어 모듈에는 제3자 런타임 의존성이 없습니다. renderer 패키지가 parser를
다시 참조하지 않으므로 parser만 쓰는 프로그램에는 renderer가 링크되지
않습니다. 최상위 패키지는 편의 조합이며 필수 진입점이 아닙니다.

## 문서 아레나

각 문서는 하이브리드 아레나를 소유합니다. `NodeID`는 연속된 node record
slice를 가리키는 32비트 인덱스이며 0은 유효하지 않습니다. 종류, 압축된
source/content span, 트리 링크, flag, 작은 정수 메타데이터는 slice에 둡니다.
선택적인 문자열과 custom payload만 sparse map에 둬 일반 문서를 작게
유지하면서 custom node도 지원합니다.

parent/first-child/last-child/previous-sibling/next-sibling 링크를 사용하므로
트리 이동은 상수 시간입니다. `Builder`는 `Build` 전까지 변경할 수 있고,
트리를 검증한 다음 동시 렌더링 가능한 읽기 전용 `Document`를 반환합니다.

공개 `Span`은 0부터 시작하는 UTF-8 byte 범위 `[Start, End)`입니다. 줄과
Unicode code-point 열 인덱스는 처음 `Position`을 호출할 때 지연 생성합니다.

## 소스 수명

| 진입점 | 소스 소유자 | 규칙 |
| --- | --- | --- |
| `Parse(ctx, []byte)` | 호출자 | document가 살아 있는 동안 byte를 변경·재사용하지 않음 |
| `ParseCopy(ctx, []byte)` | document | 호출자는 입력을 즉시 재사용 가능 |
| `ParseReader(ctx, io.Reader)` | document | 설정한 크기 제한 안에서 읽음 |
| `ast.NewBuilder(..., borrow)` | 호출자가 선택 | 위와 같은 borrow/own 계약 |

파싱은 원본을 수정하지 않습니다. 기본 입력 제한은 64 MiB이며
`WithMaxInputBytes`로 바꿀 수 있습니다. 잘못된 UTF-8, I/O 실패, 취소, 크기
제한, trace sink 실패는 fatal입니다. Markdown 복구는 해당 규칙 정책이
`Error`일 때를 제외하면 fatal error가 아니라 diagnostic으로 반환합니다.

## 파이프라인

1. context, 입력 크기, UTF-8을 검증합니다.
2. 소스 줄을 스캔하고 일치하는 block trigger를 실행해 위치가 연결된 block node를 만듭니다.
3. reference를 해석하고 inline trigger와 delimiter run을 순차 처리합니다.
4. 등록된 AST transform을 결정적인 우선순위로 실행합니다.
5. 아레나를 검증하고 동결합니다.
6. 각 renderer가 불변 document를 독립적으로 순회합니다.

trace sequence를 결정적으로 유지하기 위해 한 문서는 순차 파싱합니다. 불변
`Parser`와 renderer는 여러 문서를 동시에 처리할 수 있습니다. trace sink의
동시성은 sink가 책임지며 내장 sink는 쓰기를 직렬화합니다.

## 확장

`extension.Registry`는 중복 ID와 같은 phase·priority에서 trigger가 겹치는
등록을 거부합니다. 전역 가변 registry는 없습니다. API에는 block, inline,
transform, custom node, render 계약이 정의돼 있습니다. `parser.WithExtensions`는
parser 생성 시 block·inline·transform hook을 동결합니다. syntax handler는 제한된
context에서 양수 consumed 길이와 유효한 node를 반환해야 하며, block handler는
완전한 소스 줄을 소비해야 합니다. inline handler는 원본 소스 offset을 보며
container marker 제거로 생긴 소스 간격을 가로질러 소비할 수 없습니다.
`html.NewWithExtensions`,
`plaintext.NewWithExtensions`, `markdown.NewWithExtensions`는 parser 없이 render
handler를 컴파일합니다. render 등록 ID는 처리할 custom node 종류이고, handler는
`RenderChildren`으로 자식 순회를 제어합니다.

## 복잡도와 제한

일반 block scan과 tree walk는 입력/node 수에 선형입니다. inline emphasis는
보통 문단에는 고정 inline 저장소를, 병적인 입력에는 overflow 저장소를 쓰는
delimiter-run 계획으로 처리하며 overflow는 정렬 후 이진 탐색합니다. fuzz
test로 panic·무한루프·잘못된 span을 방지하고, 고정된 CommonMark·GFM suite가
내장 문법 계약 전체를 검증합니다.
