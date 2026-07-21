# 추적 스키마와 rule ID

[English](trace.md)

trace는 결정적인 파서 판단을 설명하며 Go 구현 세부 사항을 쏟아내는 debug
dump가 아닙니다. sink가 없으면 parser는 trace 전용 field를 만들거나 event를
보관하지 않습니다.

## Event schema v1

| 필드 | 의미 |
| --- | --- |
| `schema_version` | 현재 `1` |
| `sequence` | 한 번의 parse 안에서 1부터 시작하는 순서 |
| `level` | JSON에서는 `decisions` 또는 `verbose`에 해당하는 숫자 enum |
| `phase` | `block`, `inline`, `transform`, `render` |
| `rule_id` | 언어와 무관한 안정 식별자 |
| `decision` | `observed`, `accepted`, `rejected`, `literal`, `recovered` |
| `span` | 0부터 시작하는 UTF-8 byte half-open 범위 |
| `left`, `right` | 선택적인 인접 Unicode 문자 |
| `node_kind` | 선택적인 생성·복구 AST 종류 |
| `fields` | 순서가 있는 name/value 메타데이터 |

JSON Lines가 안정된 기계용 형식입니다. `trace.Event` 하나를 JSON 객체 한 줄로
기록합니다. 텍스트 출력은 지역화된 표현이며 `[줄:열 @byte]` 형식과 1부터
시작하는 Unicode code-point 위치를 사용합니다.

## 레벨

- `decisions`는 의미 보정, literal과 syntax 선택, 복구처럼 작성자에게 중요한
  판단을 기록합니다.
- `verbose`는 후보 구분자와 CommonMark 거부 과정도 추가로 기록합니다.

## Rule 목록

| Rule ID | 레벨 | 목적 |
| --- | --- | --- |
| `inline.delimiter.found` | verbose | delimiter run 발견 |
| `commonmark.inline.emphasis.flanking` | verbose | CommonMark flanking 문맥 허용·거부 |
| `enhance.inline.emphasis.paired-punctuation` | decisions | 등록된 쌍으로 거부된 opener를 EnhanceMark가 보정 |
| `enhance.inline.tilde.range` | decisions | 단일 물결표를 범위 구분자로 유지 |
| `enhance.inline.recovery.paragraph-end` | decisions | 닫히지 않은 inline node 복구 |
| `inline.node.created` | decisions | inline AST node 생성 |

## 예제

```sh
printf '문장**"강조"**\n' | \
  markonward explain --profile enhance --format text --locale ko --level verbose
```

로그, 편집기 연동, 회귀 fixture에는 `--format jsonl`을 사용하세요. sink 오류는
파싱을 중단하고 호출자에게 반환됩니다. 내장 collector와 writer sink는 동시
호출에 안전하지만 각 문서 event는 여전히 순차 파싱 순서로 나옵니다.
