# EnhanceMark v1 규칙

[English](enhancemark.md)

EnhanceMark v1은 현대형 GFM에 범위가 좁고 명시적인 의도 규칙을 더한
프로필입니다. 아래 규칙은 `CommonMark0312`, `GFM029`, `GFM`에서 절대
실행되지 않습니다.

## 단일 물결표

`~~텍스트~~`는 항상 GFM 취소선 동작을 유지합니다. 범위 피연산자 두 개 사이의
단일 물결표는 literal이며 단일 물결표 취소선보다 우선합니다. 현재 피연산자는
Unicode 문자나 숫자이며, 등록된 한국어·날짜·시간·단위 문자 `年 月 日 時 分 秒
개 명 번 회 층 장 권 차 주 월 일 시 분 초`도 포함합니다.

| 입력 | 해석 |
| --- | --- |
| `서울~부산` | literal 범위 구분자 |
| `9시~18시` | literal 범위 구분자 |
| `1~3명` | literal 범위 구분자 |
| `~취소선~` | 두 구분자가 내용을 감싸면 취소선 |
| `~~취소선~~` | GFM 취소선 |

범위와 취소선이 모두 가능해 보이면 범위가 우선하며
`enhance.inline.tilde.range`를 `literal` 결정으로 기록합니다.

## 짝 구두점 강조

먼저 CommonMark flanking을 판정합니다. `*` 또는 `**` 여는 구분자가 거부된
경우에도 유효한 닫는 구분자가 있고 전체 내부 내용의 첫 문자와 끝 문자가 아래
등록된 쌍일 때만 EnhanceMark가 허용합니다.

| 여는 문자 | 닫는 문자 | 여는 문자 | 닫는 문자 |
| --- | --- | --- | --- |
| `"` | `"` | `'` | `'` |
| `(` | `)` | `[` | `]` |
| `{` | `}` | `“` | `”` |
| `‘` | `’` | `「` | `」` |
| `『` | `』` | `《` | `》` |
| `〈` | `〉` | `【` | `】` |
| `（` | `）` | `［` | `］` |
| `｛` | `｝` | | |

따라서 `문장**"강조"**`는 CommonMark 여는 조건이 거부된 뒤 `Strong` node가
될 수 있습니다. `<`, `>`는 HTML 및 자동 링크와 충돌하므로 목록에서
제외했습니다. 성공한 보정은 `enhance.inline.emphasis.paired-punctuation`으로
기록합니다.

## 불완전 inline 복구

AST 종류마다 다음 정책을 설정합니다.

- `Literal`: 짝이 없는 marker를 텍스트로 유지합니다.
- `RecoverAtParagraphEnd`: 문단 끝까지 서식 node를 만들고
  `enhance.unclosed-inline` diagnostic과 recovered trace를 남깁니다.
- `Error`: byte offset을 포함한 오류로 파싱을 중단합니다.

EnhanceMark의 기본값은 emphasis, strong, strikethrough의 문단 끝 복구입니다.
code span은 명시적으로 설정하면 세 정책을 모두 지원합니다. link와 image는
없는 대상을 안전하게 추론할 수 없으므로 `Literal`과 `Error`만 지원합니다.
내용이 비어 있는 구문은 복구하지 않습니다.

복구는 inline 범위를 재귀 파싱하는 동안 적용됩니다. 따라서 중첩된 미완성
marker는 재귀 호출이 돌아오며 안쪽부터 닫힙니다. 정규화 Markdown renderer는
항상 명시적인 닫는 marker를 쓰므로 두 번째 파싱에는 다시 복구가 필요 없습니다.

## 안정성 경계

v1 릴리스 뒤에는 rule ID, diagnostic, 구두점 쌍 표, 피연산자 표가
`EnhanceMarkV1` 계약의 일부입니다. 릴리스 전 변경도 한국어·영어 trace golden
test와 프로필 차이 test를 동반해야 합니다. 폭넓은 자연어 추측은 의도적으로
범위 밖입니다. 표에 적힌 규칙을 만족하지 않는 모호한 구문은 literal로 둡니다.
