# Karyora — Kullanıcı Kılavuzu

Bu kılavuz, Karyora masaüstü uygulamasının bugün dağıtılan hâlini anlatır. Yalnızca ürünün gerçekten yaptığını belirtir; henüz var olmayan yetenekler sınırlamalar olarak listelenir, asla vaat olarak sunulmaz. Aynı kılavuz bu klasörde 12 dilde mevcuttur.

## 1. Karyora nedir

- Karyora, yerel öncelikli (local-first) bir masaüstü uygulamasıdır. Bilgi paketleriniz, işleriniz (job) ve ayarlarınız kendi makinenizde saklanır.
- Ürün, projenin hiçbir bulut hizmeti olmadan çalışır. Üretebileceği tek ağ trafiği, sizin başlattığınız traafiktir: AI sağlayıcılarınıza giden istekler, talep ettiğiniz bir bilgi paketi içe aktarma ve — yapılandırırsanız — güncelleme kanalı.
- Yapay zekâ özellikleri kendi sağlayıcı hesaplarınızı kullanır (BYOK — bring your own key). API anahtarlarınızı siz yapılandırırsınız ve ücreti sağlayıcı doğrudan sizden tahsil eder. Karyora, sağlayıcı kullanımını asla yeniden satmaz ve ödemelerde aracı olarak rol almaz.

## 2. Topluluk ve resmî sürümler

- Karyora'nın kaynak kodu Apache License 2.0 ile yayımlanır. Bu lisans kapsamında onu kullanabilir, inceleyebilir, değiştirebilir ve yeniden dağıtabilirsiniz — kendi başınıza derlemek dahil.
- Genel depo `https://github.com/qira-dev/karyora` adresidir. Her sürüm bir `v*` etiketiyle işaretlenir ve o etiketin kaynak ağacı o sürümün kaynağıdır.
- "Resmî" sürüm; projenin kendi derleme hattı tarafından, etiketli bir revizyonda ve kayıtlı kanıtlarla derlenmiş sürümdür. Resmî ve topluluk derlemeleri aynı özelliklere sahiptir; fark, yapıtı kimin derlediğinde ve kimin sorumluluğundadır — yapıtın ne yapabildiğinde değildir.
- v1'de henüz resmî bir indirme sayfası ve resmî bir güncelleme kanalı yoktur. Bunlar yayımlanana kadar Karyora'yı genel depodan alıp kaynaktan derleyebilirsiniz.

## 3. Desteklenen platformlar (v1)

| İşletim sistemi | CPU | Alt sınır |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 veya yenisi |
| Windows 11 | ARM64 | ARM için Windows 11 |
| macOS | x86_64 | macOS 15 (Sequoia) veya yenisi |
| macOS | ARM64 | macOS 15 (Sequoia) veya yenisi, Apple Silicon |
| Linux | x86_64 | glibc 2.34 veya yenisi; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 veya yenisi; aynı dağıtım aileleri |

v1'de açıkça desteklenmeyenler: Windows 10; macOS 14 ve öncesi; Alpine gibi musl tabanlı dağıtımlar; yukarıda listelenmeyen her kombinasyon. Destek, her tedarikçinin kendi güvenlik desteği penceresini izler ve her sürümde yeniden kontrol edilir.

## 4. Paketler ve doğrulama

- Üretilen paket kümesi şudur: Windows MSI, macOS pkg, Linux deb, Linux rpm ve taşınabilir bir Linux arşivi — desteklenen her CPU mimarisi için bir yapıt.
- Her paket, bir `SHA256SUMS` bildirisiyle ve yapıt başına SBOM ile kaynak (provenance) kayıtlarıyla birlikte üretilir. Bir paketi doğrulamak için: indirdiğiniz dosyanın sha256 özetini hesaplayın ve bildirideki girdiyle karşılaştırın. Özet eşleşmiyorsa paketi kurmayın.
- Paketler v1'de tasarım gereği dijital olarak imzalanmamıştır. Platform düzeyinde imza doğrulama adımları (işletim sistemi imza kontrolleri, depo anahtarları) ilk imzalı sürümle birlikte belgelenecektir. O zamana dek yapabileceğiniz doğrulama, yukarıdaki özet karşılaştırmasıdır.
- Proje lisansını ve üçüncü taraf atıflarını taşıyan `LICENSE` ve `NOTICE` dosyaları, kaynak deposunun bir parçasıdır.

## 5. Güncellemeler

- Karyora bir güncelleme mekanizması içerir ve v1'de bu mekanizma varsayılan olarak devre dışı dağıtılır. Hiçbir güncelleme kanalı yapılandırılmadığı için güncelleme denetimi, "endpoint yapılandırılmamış" diyen tipli bir reddetme bildirir. Bu kasıtlıdır: resmî güncelleme kanalı ancak yayımlandığında açılır.
- O zamana dek güncelleme, daha yeni paketi eskinin üzerine kurmak demektir. Güncellemeler sırasında verileriniz korunur (bkz. bölüm 6).

## 6. Verileriniz

- Ürün verilerinin tümü sizin makinenizdedir. Varsayılan veri dizini: Windows'ta `%APPDATA%\Karyora\data`, macOS'ta `~/Library/Application Support/Karyora/data`, Linux'ta `~/.local/share/karyora/data` (XDG `XDG_DATA_HOME` ayarına saygı gösterilir). `--data-dir` bayrağıyla başka bir konum seçebilirsiniz.
- Ürün zamanlanmış yedekleme yapmaz. Yedeklemek için: ürünü durdurun ve veri dizinini kopyalayın. Geri yükleme, kopyalanan dizini yerine geri koymaktır. Bulut yedeği yoktur.
- Kaldırma veya güncelleme verilerinizi silmez. Veri silmek, sizin kendi başınıza yaptığınız ayrı ve açık bir işlemdir.
- Bekleyen veriler (data at rest) şifrelenmez. Veri dizinini kendi araçlarınızla koruyun — örneğin tam disk şifrelemesi. Bu, açıkça belirtilmiş bilinen bir sınırlamadır.
- Sağlayıcı kimlik bilgileri (API anahtarlarınız) yalnızca işletim sisteminizin anahtarlığında (keyring) saklanır. Bunlar asla dosyalara, günlüklere veya istem içeriğine yazılmaz.

## 7. Gizlilik

- Ürün telemetri içermez: projenin sunucularına hiçbir şey gönderilmez.
- Tanılama verileri yerel olarak üretilir ve varsayılan olarak gizliliği korur; makinenizden bir şey çıkıp çıkmayacağına siz karar verirsiniz.

## 8. v1'de bilinen sınırlamalar

- Üçüncü taraf eklenti çalıştırma devre dışıdır.
- Takım ve uzak çalışma kullanılamaz.
- Çalışan bir işi başka bir makineye aktarmak desteklenmez.
- Taşınabilir bilgi paketi eşitleme kullanılamaz.
- "Work" gruplama özelliği yoktur.
- Ücretli abonelik ve hak (entitlement) yoktur: satın alınacak bir şey, ücretli kademe yoktur.
- Sayısal performans vaadi yapılmaz.
- Bekleyen veriler şifrelenmez (bkz. bölüm 6).
- Henüz resmî indirme sayfası ve güncelleme kanalı yoktur (bkz. bölüm 2 ve 5).

## 9. Lisans

- Karyora Apache License 2.0 ile yayımlanır. Lisans metninin tamamı kaynak deposundaki `LICENSE` dosyasındadır; üçüncü taraf atıfları `NOTICE` dosyasındadır.
- Karyora'yı kendiniz derlemek, fork etmek veya lisans koşullarıyla yeniden dağıtmak, lisansın tanıdığı tüm hakları korur. Bu hak hiçbir satın almaya bağlı değildir; çünkü satın alınacak bir şey yoktur.

## 10. Komut satırı başvurusu

Komut satırı aracı `uawp`'dir.

| Komut | Yaptığı iş |
|---|---|
| `uawp start` | Yerel hizmeti başlatır. Aynı veri dizini üzerinde biri çalışırken ikinci bir start reddedilir. |
| `uawp stop` | Hizmeti sınırlı ve düzenli bir kapanışla durdurur. |
| `uawp status` | Hizmetin çalışıp çalışmadığını bildirir. |
| `uawp doctor` | Yerel sağlık ve tanılama kontrolleri yapar. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Güncellemeleri denetler, uygular veya bildirir. v1'de kanal yapılandırılmadığı için check ve apply, "endpoint yapılandırılmamış" diyen tipli bir reddetmeyle biter; `status` güncelleme durumunu bildirir. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Kalıcı bir iş gönderir, durumunu bildirir veya olaylarını izler. |
| `uawp credential store --provider <id> [--name api-key]` | Sağlayıcı API anahtarınızı işletim sistemi anahtarlığına kaydeder. |
| `uawp credential status` | Kimlik bilgisinin var olup olmadığını bildirir — değerini asla göstermez. |
| `uawp credential remove` | Saklanan kimlik bilgisini kaldırır. |
| `uawp version` / `uawp help` | Sürümü veya yardımı yazdırır. |

Saklanmış bir kimlik bilgisi tek başına hiçbir şey açmaz: sağlayıcıya gerçek bir çağrının, ürünün o sağlayıcı için uyguladığı veri işleme kurallarından geçmesi gerekir.
