import AVFoundation

/// AVAudioRecorder 包装。录到 m4a (AAC) 16kHz mono 临时文件。
final class Recorder {
    private var recorder: AVAudioRecorder?
    private(set) var currentURL: URL?

    /// 开始录音。返回 false 表示初始化失败 (常见原因:无 mic 权限)。
    func start() -> Bool {
        let url = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("panel-voice-\(Int(Date().timeIntervalSince1970)).m4a")
        let settings: [String: Any] = [
            AVFormatIDKey: Int(kAudioFormatMPEG4AAC),
            AVSampleRateKey: 16000,
            AVNumberOfChannelsKey: 1,
            AVEncoderAudioQualityKey: AVAudioQuality.medium.rawValue
        ]
        do {
            let r = try AVAudioRecorder(url: url, settings: settings)
            guard r.record() else { return false }
            self.recorder = r
            self.currentURL = url
            return true
        } catch {
            return false
        }
    }

    /// 停止并返回文件 URL。
    func stop() -> URL? {
        guard let r = recorder else { return nil }
        r.stop()
        let url = currentURL
        recorder = nil
        currentURL = nil
        return url
    }
}
