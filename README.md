# GoTictok

Ứng dụng Go phát triển theo quy trình **ECC** ([affaan-m/ECC](https://github.com/affaan-m/ECC)),
có **Graphify** ([Graphify-Labs/graphify](https://github.com/Graphify-Labs/graphify))
làm bản đồ tri thức mã nguồn cho các AI agent.

## Bắt đầu

Yêu cầu: Go 1.27+, Git, `gcc` (cho `go test -race`), và Graphify
(`uv tool install graphifyy`).

```bash
go run ./tools/eccgate setup    # bật git hooks, cài linter đúng phiên bản, đăng ký merge driver
go run ./tools/eccgate doctor   # kiểm tra clone đã sẵn sàng
```

Mỗi clone mới phải chạy `setup` một lần: git không mang cấu hình hooks theo repo.

## Quy trình bắt buộc

Mọi agent (và người) đều làm theo [AGENTS.md](AGENTS.md):

1. **Research**: hỏi knowledge graph trước (`graphify query "..."`), tìm thứ có sẵn trước khi viết mới.
2. **Plan**: lập kế hoạch cho việc không tầm thường.
3. **TDD**: viết test thất bại trước (RED), code tối thiểu (GREEN), dọn dẹp (REFACTOR).
4. **Review**: tự rà soát theo vai `go-reviewer`, `code-reviewer`, `security-reviewer`.
5. **Verify**: `go run ./tools/eccgate verify` phải in `RESULT: READY`.
6. **Commit**: `<type>: <mô tả>` (feat, fix, refactor, docs, test, chore, perf, ci, build, style, revert).

## Hai rule riêng của dự án

Ngoài bộ rule ECC, dự án có hai rule trong `.claude/rules/project/` (tóm tắt ở
mục 9 và 10 của [AGENTS.md](AGENTS.md)):

- **[Tiêu chuẩn công nghiệp](.claude/rules/project/industrial-standards.md)**:
  mã phải đủ tốt để chạy production không người trông. Chuẩn tham chiếu là
  Effective Go, Google và Uber Go Style Guide, Twelve-Factor, OWASP ASVS. Phần
  đo được (xử lý lỗi, context, hằng số có tên, kích thước hàm, interface nhỏ,
  bảo mật) do `golangci-lint` chặn với 56 linter; phần còn lại (timeout cho
  mọi lời gọi ra ngoài, goroutine có chủ, graceful shutdown, log có cấu trúc,
  cấu hình được kiểm tra khi khởi động) do các vai review kiểm.
- **[Nghiên cứu khi gặp vấn đề khó](.claude/rules/project/research-first.md)**:
  khi sửa hai lần không được, chưa hiểu nguyên nhân gốc, hay phải chọn kiến
  trúc hoặc thư viện, agent phải dừng code và làm đủ 7 bước: nêu rõ vấn đề,
  tìm trong dự án, tìm ít nhất 3 nguồn bên ngoài (tài liệu gốc, cách các dự án
  Go lớn giải quyết, pkg.go.dev), đánh giá, **tổng hợp** thành một khuyến
  nghị, ghi lại ở [docs/research/](docs/research/README.md), rồi mới **lập kế
  hoạch**. Trong Claude Code: `/research <vấn đề>`.

Mọi dependency trực tiếp trong `go.mod` phải có dòng tương ứng trong
[docs/research/dependencies.md](docs/research/dependencies.md); thiếu thì kiểm
tra `deps` của cổng báo lỗi.

## Cổng kiểm tra `eccgate`

Chỉ dẫn bằng văn bản không thể ép một AI agent tuân thủ, nên mọi quy tắc kiểm
được bằng máy đều nằm trong `tools/eccgate` và chạy ở ba nơi:

| Nơi chạy | Khi nào | Bỏ qua được không |
|----------|---------|-------------------|
| Git hooks (`.githooks/`) | mỗi commit và push, với mọi agent | được, bằng `--no-verify` |
| Hooks của Claude Code (`.claude/settings.json`) | sau mỗi lần sửa file, khi agent định dừng, khi chạy lệnh | chặn `--no-verify`; sửa file của cổng phải có người duyệt |
| CI (`.github/workflows/ecc-gate.yml`) | mỗi push và pull request | không |

Các kiểm tra: `gofmt`, `go mod tidy`, sổ dependency (`deps`), build, `go vet`,
`golangci-lint` (bọc lỗi bằng `%w`, không bỏ qua lỗi, không số ma thuật, hàm
dưới 50 dòng, file dưới 800 dòng, lồng không quá 4 cấp, không biến toàn cục,
interface nhỏ, `gosec`), mọi package đều có test, `go test -race` với coverage
tối thiểu 80%, không secrets trong mã, `govulncheck`, định dạng commit.

```bash
go run ./tools/eccgate verify --quick   # bản nhanh, giống pre-commit
go run ./tools/eccgate verify           # bản đầy đủ, giống pre-push và CI
```

### Để thật sự không ai lách được

Hooks cục bộ là lớp phản hồi nhanh; lớp chốt chặn là CI. Trên GitHub, hãy bật
**branch protection** cho `main`: bắt buộc status check `ECC gate / verify`,
bắt buộc pull request và review từ code owner. File
[.github/CODEOWNERS](.github/CODEOWNERS) đã gán các file của cổng
(`tools/eccgate/`, `.golangci.yml`, `.githooks/`, `.github/`, `AGENTS.md`,
`.claude/rules/`) cho chủ repo. Khi đó code không đạt chuẩn không thể vào
`main`, và không agent nào tự nới lỏng được cổng.

## Git

- Remote: `origin` = <https://github.com/LeNguySon/GoTictok>, nhánh chính `main`.
- Làm việc trên nhánh riêng, mở pull request theo
  [mẫu có sẵn](.github/PULL_REQUEST_TEMPLATE.md); commit theo Conventional Commits.
- `go run ./tools/eccgate setup` đặt các cấu hình git của riêng từng clone
  (hooks, merge driver cho graph). `.gitattributes` giữ line ending là LF,
  `.editorconfig` thống nhất định dạng giữa các editor.
- Dependabot ([.github/dependabot.yml](.github/dependabot.yml)) mở PR cập nhật
  Go module và GitHub Actions hằng tuần.

Hai bước Plan và Review không kiểm được bằng máy; chúng dựa vào agent và người review.

## Knowledge graph (Graphify)

```bash
graphify query "luồng xử lý của lệnh version đi qua đâu?"
graphify explain "Run"
graphify affected "Run"      # sửa symbol này thì ảnh hưởng tới đâu
graphify update .            # dựng lại graph từ mã nguồn (cục bộ, không cần LLM)
```

- `graphify-out/graph.json` được commit để mọi agent dùng chung một bản đồ;
  pre-commit hook tự làm mới và stage file này.
- Phần còn lại của `graphify-out/` (báo cáo, `graph.html`, cache) chỉ nằm trên máy.
- Mở `graphify-out/graph.html` để xem graph trực quan; `GRAPH_REPORT.md` là tổng quan kiến trúc.

## Cấu trúc

```
cmd/gotictok/          điểm vào của ứng dụng
internal/              mã nghiệp vụ
tools/eccgate/         cổng kiểm tra (Go, có test)
.githooks/             git hooks, gọi eccgate
.claude/               rules, agents, commands, skills của ECC + hooks
.cursor/ .codex/ .gemini/ .copilot/ .agents/   adapter cho từng AI agent
AGENTS.md              hợp đồng chung cho mọi agent
docs/research/         ghi chú nghiên cứu và sổ dependency
graphify-out/          knowledge graph
```

## Nâng cấp ECC

Phần ECC được vendor ở phiên bản ghi trong `.claude/ecc-vendor.json`. Để cập nhật:

```bash
git clone --depth 1 https://github.com/affaan-m/ECC.git /tmp/ecc
sh scripts/vendor-ecc.sh /tmp/ecc
```

Rồi xem lại diff như mọi thay đổi khác.
