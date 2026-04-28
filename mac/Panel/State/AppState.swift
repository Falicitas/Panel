import Foundation
import Observation

@MainActor
@Observable
final class AppState {
    var selectedTab: AppTab = .todolist
    var active: [ListEntry] = []
    var done: [ListEntry] = []
    var anchor: String?
    var focus: FocusResult?
    var detail: Todolist?
    var microToday: Todolist?
    var recurring: [RecurringTodo] = []
    var isLoading = false
    var lastError: String?

    private let api = APIClient.shared
    private var pollTask: Task<Void, Never>?

    func loadAll() async {
        isLoading = true
        defer { isLoading = false }
        do {
            async let lists = api.listTodolists()
            async let anchorResp = api.getAnchor()
            async let focusResp = api.getFocus()
            async let microResp = api.getMicroToday()
            async let recurResp = api.listRecurring()
            let (l, a, f, m, r) = try await (lists, anchorResp, focusResp, microResp, recurResp)
            self.active = l.active
            self.done = l.done
            self.anchor = a.name
            self.focus = f
            self.microToday = m
            self.recurring = r
            if let name = a.name {
                self.detail = try await api.getTodolist(name: name)
            } else {
                self.detail = nil
            }
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveMicroToday(rawMD: String) async {
        do {
            self.microToday = try await api.putMicroToday(rawMD: rawMD)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func addRecurring(content: String) async {
        let nextOrder = (recurring.last?.sortOrder ?? -1) + 1
        do {
            _ = try await api.createRecurring(content: content, sortOrder: nextOrder)
            self.recurring = try await api.listRecurring()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteRecurring(_ item: RecurringTodo) async {
        do {
            try await api.deleteRecurring(id: item.id)
            self.recurring = try await api.listRecurring()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func setAnchor(name: String?) async {
        do {
            _ = try await api.setAnchor(name: name)
            self.anchor = name
            if let n = name {
                async let detail = api.getTodolist(name: n)
                async let focus = api.getFocus()
                let (d, f) = try await (detail, focus)
                self.detail = d
                self.focus = f
            } else {
                self.detail = nil
                self.focus = try? await api.getFocus()
            }
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveDetail(rawMD: String) async {
        guard let name = detail?.name else { return }
        do {
            self.detail = try await api.putTodolist(name: name, rawMD: rawMD)
            self.focus = try await api.getFocus()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 后台 polling: 处理外部编辑器(Typora 等)修改了文件的情况。
    /// 后端 fsnotify 已经失效缓存,这里只是周期性拉取看是否有新内容。
    func startPolling() async {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(3))
                await self?.tick()
            }
        }
        await pollTask?.value
    }

    private func tick() async {
        do {
            if let name = anchor {
                async let detail = api.getTodolist(name: name)
                async let focus = api.getFocus()
                let (d, f) = try await (detail, focus)
                if d != self.detail { self.detail = d }
                if f != self.focus { self.focus = f }
            }
            let m = try await api.getMicroToday()
            if m != self.microToday { self.microToday = m }
        } catch {
            // 静默,等下次
        }
    }
}
