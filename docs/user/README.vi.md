# Karyora — Hướng dẫn sử dụng

Hướng dẫn này mô tả ứng dụng desktop Karyora đúng như phiên bản đang được phân phối hôm nay. Nó chỉ nêu những gì sản phẩm thực sự làm được; những khả năng chưa tồn tại được liệt kê là giới hạn, không bao giờ được trình bày như lời hứa. Cùng hướng dẫn này có bản 12 ngôn ngữ trong thư mục này.

## 1. Karyora là gì

- Karyora là ứng dụng desktop ưu tiên dữ liệu cục bộ (local-first). Các gói tri thức, công việc (job) và cài đặt của bạn được lưu trên chính máy của bạn.
- Sản phẩm hoạt động mà không cần bất kỳ dịch vụ đám mây nào của dự án. Lưu lượng mạng duy nhất mà nó có thể tạo ra là lưu lượng do bạn khởi tạo: các yêu cầu tới nhà cung cấp AI của bạn, một lần nhập gói tri thức mà bạn yêu cầu, và — nếu bạn cấu hình — kênh cập nhật.
- Các tính năng AI dùng tài khoản nhà cung cấp của chính bạn (BYOK — bring your own key). Bạn tự cấu hình khóa API của mình, và nhà cung cấp trực tiếp tính phí bạn. Karyora không bao giờ bán lại lượng dùng của nhà cung cấp và không bao giờ làm trung gian thanh toán cho họ.

## 2. Bản dựng cộng đồng và bản chính thức

- Mã nguồn của Karyora được phát hành theo giấy phép Apache License 2.0. Bạn có thể sử dụng, nghiên cứu, sửa đổi và phân phối lại theo giấy phép đó, kể cả tự build.
- Kho mã công khai là `https://github.com/qira-dev/karyora`. Mỗi bản phát hành được đánh dấu bằng thẻ `v*`, và cây mã nguồn của thẻ đó chính là mã nguồn của bản phát hành.
- Bản "chính thức" là bản do chính pipeline build của dự án build tại một revision có gắn thẻ, với bằng chứng được ghi lại. Bản chính thức và bản cộng đồng có cùng tính năng; sự khác biệt nằm ở ai đã build artifact và ai chịu trách nhiệm về nó — không bao giờ nằm ở việc artifact làm được gì.
- Ở v1 chưa có trang tải xuống chính thức và chưa có kênh cập nhật chính thức. Cho đến khi chúng được công bố, bạn có thể lấy Karyora từ kho mã công khai và tự build từ mã nguồn.

## 3. Nền tảng được hỗ trợ (v1)

| Hệ điều hành | CPU | Ngưỡng tối thiểu |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 hoặc mới hơn |
| Windows 11 | ARM64 | Windows 11 cho ARM |
| macOS | x86_64 | macOS 15 (Sequoia) hoặc mới hơn |
| macOS | ARM64 | macOS 15 (Sequoia) hoặc mới hơn, Apple Silicon |
| Linux | x86_64 | glibc 2.34 hoặc mới hơn; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 hoặc mới hơn; cùng các dòng bản phân phối đó |

Không được hỗ trợ trong v1: Windows 10; macOS 14 hoặc cũ hơn; các bản phân phối dựa trên musl như Alpine; mọi tổ hợp không được liệt kê ở trên. Việc hỗ trợ đi theo cửa sổ hỗ trợ bảo mật của từng nhà sản xuất và được kiểm tra lại ở mỗi bản phát hành.

## 4. Gói cài đặt và cách xác minh

- Bộ gói được tạo ra gồm: MSI của Windows, pkg của macOS, deb của Linux, rpm của Linux và một kho lưu trữ di động của Linux — mỗi kiến trúc CPU được hỗ trợ một artifact.
- Mỗi gói được tạo kèm với tệp kê khai `SHA256SUMS` cùng hồ sơ SBOM và provenance cho từng artifact. Cách xác minh một gói: tính giá trị băm sha256 của bản bạn tải về và so với mục trong tệp kê khai. Nếu giá trị băm không khớp, đừng cài đặt gói đó.
- Các gói ở v1 không được ký số — đúng theo thiết kế. Các bước xác minh chữ ký ở mức hệ điều hành (kiểm tra chữ ký của hệ điều hành, khóa của kho phần mềm) sẽ được ghi lại cùng với bản phát hành có ký tự đầu tiên. Trước đó, việc so giá trị băm nêu trên chính là cách xác minh bạn có thể thực hiện.
- Các tệp `LICENSE` và `NOTICE` — nơi chứa giấy phép của dự án và phần ghi nhận các thành phần bên thứ ba — là một phần của kho mã nguồn.

## 5. Cập nhật

- Karyora có sẵn cơ chế cập nhật và ở v1 nó được phân phối ở trạng thái tắt theo mặc định. Không có kênh cập nhật nào được cấu hình, nên khi kiểm tra cập nhật, chương trình báo một lần từ chối có kiểu, nêu rõ rằng chưa có endpoint nào được cấu hình. Đây là hành vi chủ đích: kênh cập nhật chính thức chỉ được bật khi nó được công bố.
- Trước đó, nâng cấp nghĩa là cài gói mới hơn lên trên gói cũ. Dữ liệu của bạn được giữ nguyên qua các lần nâng cấp (xem mục 6).

## 6. Dữ liệu của bạn

- Toàn bộ dữ liệu của sản phẩm nằm trên máy của bạn. Thư mục dữ liệu mặc định là `%APPDATA%\Karyora\data` trên Windows, `~/Library/Application Support/Karyora/data` trên macOS và `~/.local/share/karyora/data` trên Linux (tôn trọng thiết lập XDG `XDG_DATA_HOME`). Bạn có thể chọn vị trí khác bằng cờ `--data-dir`.
- Sản phẩm không thực hiện sao lưu định kỳ. Để sao lưu: dừng sản phẩm rồi sao chép thư mục dữ liệu. Khôi phục nghĩa là đưa thư mục đã sao chép trở lại vị trí cũ. Không có sao lưu đám mây.
- Gỡ cài đặt hoặc nâng cấp không xóa dữ liệu của bạn. Việc xóa dữ liệu là một hành động riêng biệt, rõ ràng, do bạn tự thực hiện.
- Dữ liệu khi lưu trên đĩa không được mã hóa. Hãy tự bảo vệ thư mục dữ liệu bằng phương tiện của bạn, ví dụ mã hóa toàn bộ ổ đĩa. Đây là giới hạn đã biết, được nêu thẳng thắn.
- Thông tin xác thực của nhà cung cấp (khóa API của bạn) chỉ được lưu trong keyring của hệ điều hành. Chúng không bao giờ được ghi vào tệp, nhật ký hay nội dung câu lệnh (prompt).

## 7. Quyền riêng tư

- Sản phẩm không có telemetry: không có gì được gửi tới máy chủ của dự án.
- Dữ liệu chẩn đoán được tạo cục bộ và mặc định tôn trọng quyền riêng tư; bạn quyết định có bất kỳ gì rời khỏi máy của mình hay không.

## 8. Giới hạn đã biết ở v1

- Việc chạy plugin của bên thứ ba đang bị tắt.
- Chế độ nhóm (Team) và chế độ từ xa không có sẵn.
- Chuyển một công việc đang chạy sang máy khác không được hỗ trợ.
- Đồng bộ gói tri thức di động không có sẵn.
- Tính năng nhóm "Work" không tồn tại.
- Không có gói đăng ký trả phí và không có quyền (entitlement): không có gì để mua, không có bậc trả phí.
- Không đưa ra bất kỳ lời hứa hiệu năng nào bằng con số.
- Dữ liệu khi lưu trên đĩa không được mã hóa (xem mục 6).
- Chưa có trang tải xuống chính thức và chưa có kênh cập nhật (xem mục 2 và 5).

## 9. Giấy phép

- Karyora được phát hành theo giấy phép Apache License 2.0. Toàn văn giấy phép nằm trong tệp `LICENSE` và phần ghi nhận thành phần bên thứ ba nằm trong tệp `NOTICE`; cả hai đều nằm trong kho mã nguồn.
- Tự build Karyora, fork nó hay phân phối lại theo giấy phép đều giữ nguyên mọi quyền mà giấy phép trao. Quyền này không phụ thuộc vào việc mua bất cứ thứ gì, vì vốn không có gì để mua.

## 10. Tham chiếu dòng lệnh

Công cụ dòng lệnh là `uawp`.

| Lệnh | Tác dụng |
|---|---|
| `uawp start` | Khởi động dịch vụ cục bộ. Lần start thứ hai trên cùng thư mục dữ liệu sẽ bị từ chối khi một bản đang chạy. |
| `uawp stop` | Dừng dịch vụ bằng quy trình tắt có giới hạn, trọn vẹn. |
| `uawp status` | Báo cáo dịch vụ có đang chạy hay không. |
| `uawp doctor` | Chạy các kiểm tra tình trạng và chẩn đoán cục bộ. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Kiểm tra, áp dụng hoặc báo cáo cập nhật. Ở v1, khi chưa cấu hình kênh, check và apply kết thúc bằng một lần từ chối có kiểu, nêu rõ chưa cấu hình endpoint; `status` báo trạng thái cập nhật. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Gửi một công việc bền vững, báo trạng thái của nó hoặc theo dõi các sự kiện của nó. |
| `uawp credential store --provider <id> [--name api-key]` | Lưu khóa API của nhà cung cấp vào keyring của hệ điều hành. |
| `uawp credential status` | Báo cáo thông tin xác thực có tồn tại hay không — không bao giờ hiển thị giá trị của nó. |
| `uawp credential remove` | Xóa thông tin xác thực đã lưu. |
| `uawp version` / `uawp help` | In phiên bản hoặc trợ giúp. |

Chỉ lưu thông tin xác thực thì chưa mở được gì: một cuộc gọi thật tới nhà cung cấp vẫn phải qua các quy tắc xử lý dữ liệu mà sản phẩm áp dụng cho nhà cung cấp đó.
