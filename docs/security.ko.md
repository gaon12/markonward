# 보안 모델

[English](security.md)

Markdown는 기본적으로 신뢰하지 않는 입력입니다. Markonward의 안전 HTML
renderer는 byte가 출력 writer에 도달하기 전에 구조적인 방어 두 종류를
적용합니다.

## 기본 HTML 동작

- raw inline/block HTML을 escape합니다.
- 제어 문자와 공백을 제거한 뒤 URL scheme을 해석합니다.
- 상대 URL과 `http`, `https`, `mailto`, `tel`, `ftp` scheme만 허용합니다.
  `javascript`, `vbscript`, `file`, `data`를 포함한 그 밖의 명시적 scheme은
  빈 목적지로 바꿉니다.
- HTML 텍스트, attribute, title, code, alt text를 각 출력 문맥에 맞게
  escape합니다.

`html.WithUnsafe()`는 명세 test 또는 애플리케이션이 이미 신뢰한 입력에만
씁니다. raw HTML과 위험 scheme을 허용합니다. 그래도 GFM 계열 프로필 문서는
`title`, `textarea`, `style`, `xmp`, `iframe`, `noembed`, `noframes`, `script`,
`plaintext` tag에 GFM tagfilter를 적용합니다.

unsafe mode는 HTML sanitizer가 아닙니다. 다른 신뢰 정책이 필요하면 safe
mode를 유지하거나 이 무의존성 core 밖에서 정책 기반 sanitizer를 적용하세요.

## 자원 제어

- 입력은 올바른 UTF-8이어야 합니다.
- parser 입력 기본 최대값은 64 MiB이며 `WithMaxInputBytes`로 낮출 수 있습니다.
- block/inline 작업과 rendering 도중 `context.Context` 취소를 확인합니다.
- fuzz target은 block, delimiter, link, table, 모든 renderer, 정규화 왕복을
  다룹니다.
- document를 반환하기 전에 arena span과 관계를 검증합니다.

현재 v1 이전 inline 구현은 모든 적대적 delimiter 패턴에서 선형임이 아직
증명되지 않았습니다. fuzzing 뒤에도 입력 제한과 요청 deadline을 배포 정책의
일부로 사용하세요.

## 소스 소유권과 data race

`Parse`는 의도적으로 호출자 메모리를 빌립니다. document를 파싱·렌더링하는
동안 byte를 바꾸면 호출자 측 data race이며 출력이 손상될 수 있습니다. 소유권
경계를 넘을 때는 `ParseCopy` 또는 `ParseReader`를 사용하세요. 그 밖에는
document와 불변 parser/renderer 설정을 goroutine 사이에 공유할 수 있습니다.

## Trace 개인정보

trace event에는 byte span, 인접 문자, field의 destination/kind, 지역화된 위치
출력이 들어갑니다. 따라서 sink가 비공개 Markdown 조각을 노출할 수 있습니다.
애플리케이션 로그와 같은 접근 제어·보존·마스킹 정책을 적용하세요. 설명이 실제로
필요하지 않은 민감한 hot path에서는 trace를 끄세요.

## 신고

악용 가능한 세부 사항을 일반 issue에 공개하지 마세요. 활성화된 뒤에는
`gaon12/markonward`의 GitHub private vulnerability reporting을 사용하세요.
그 전에는 공개 전에 저장소 소유자에게 비공개로 연락하세요.
