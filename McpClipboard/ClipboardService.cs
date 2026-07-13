#if WINDOWS
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Windows.Forms;
#endif
using Microsoft.Extensions.Logging;

namespace McpClipboard;

public class ClipboardService : IDisposable
{
    private readonly ILogger<ClipboardService> _logger;
#if WINDOWS
    private ClipboardNotificationForm? _form;
    private Thread? _formThread;
#endif
    private volatile bool _isDisposed;
    
    public event EventHandler<ClipboardChangedEventArgs>? ClipboardChanged;

    public ClipboardService(ILogger<ClipboardService> logger)
    {
        _logger = logger;
    }

    public async Task StartMonitoringAsync()
    {
        if (OperatingSystem.IsWindows())
        {
#if WINDOWS
            if (_formThread != null) return;

            // Check if we're already in STA
            if (Thread.CurrentThread.GetApartmentState() == ApartmentState.STA)
            {
                // We're in STA, can create form directly
                try
                {
                    Application.SetHighDpiMode(HighDpiMode.SystemAware);
                    Application.EnableVisualStyles();
                    Application.SetCompatibleTextRenderingDefault(false);

                    _form = new ClipboardNotificationForm(_logger);
                    _form.ClipboardChanged += OnClipboardChanged;
                    
                    _logger.LogInformation("Clipboard monitoring started on main STA thread");
                }
                catch (Exception ex)
                {
                    _logger.LogError(ex, "Error creating clipboard form on STA thread");
                    throw;
                }
            }
            else
            {
                // Create STA thread for Windows Forms
                var tcs = new TaskCompletionSource();
                
                _formThread = new Thread(() =>
                {
                    try
                    {
                        Application.SetHighDpiMode(HighDpiMode.SystemAware);
                        Application.EnableVisualStyles();
                        Application.SetCompatibleTextRenderingDefault(false);

                        _form = new ClipboardNotificationForm(_logger);
                        _form.ClipboardChanged += OnClipboardChanged;
                        
                        tcs.SetResult();
                        Application.Run(_form);
                    }
                    catch (Exception ex)
                    {
                        _logger.LogError(ex, "Error in clipboard monitoring thread");
                        tcs.SetException(ex);
                    }
                })
                {
                    IsBackground = true,
                    Name = "ClipboardMonitor"
                };

                _formThread.SetApartmentState(ApartmentState.STA);
                _formThread.Start();
                
                await tcs.Task;
                _logger.LogInformation("Clipboard monitoring started on separate STA thread");
            }
#else
            await Task.CompletedTask;
#endif
        }
        else
        {
            _logger.LogWarning("Clipboard monitoring is only supported on Windows");
            await Task.CompletedTask;
        }
    }

#if WINDOWS
    private void OnClipboardChanged(object? sender, ClipboardChangedEventArgs e)
    {
        ClipboardChanged?.Invoke(this, e);
    }
#endif

    public void Dispose()
    {
        if (_isDisposed) return;
        _isDisposed = true;

#if WINDOWS
        if (_form != null)
        {
            _form.ClipboardChanged -= OnClipboardChanged;
            _form.Invoke(() => _form.Close());
        }

        _formThread?.Join(1000);
#endif
        GC.SuppressFinalize(this);
    }
}

public class ClipboardChangedEventArgs : EventArgs
{
    public ClipboardContentType ContentType { get; set; }
    public int Size { get; set; }
    public string? Preview { get; set; }
    public DateTime Timestamp { get; set; } = DateTime.Now;
}

public enum ClipboardContentType
{
    Empty,
    Text,
    Image,
    Files,
    Other
}

internal class ClipboardNotificationForm : Form
{
    private readonly ILogger _logger;
    private IntPtr _nextClipboardViewer;

    public event EventHandler<ClipboardChangedEventArgs>? ClipboardChanged;

    public ClipboardNotificationForm(ILogger logger)
    {
        _logger = logger;
        
        // Make form invisible
        WindowState = FormWindowState.Minimized;
        ShowInTaskbar = false;
        Visible = false;
        
        // Register for clipboard notifications
        _nextClipboardViewer = SetClipboardViewer(Handle);
    }

    protected override void WndProc(ref Message m)
    {
        const int WM_DRAWCLIPBOARD = 0x0308;
        const int WM_CHANGECBCHAIN = 0x030D;

        switch (m.Msg)
        {
            case WM_DRAWCLIPBOARD:
                OnClipboardContentChanged();
                SendMessage(_nextClipboardViewer, m.Msg, m.WParam, m.LParam);
                break;

            case WM_CHANGECBCHAIN:
                if (m.WParam == _nextClipboardViewer)
                    _nextClipboardViewer = m.LParam;
                else
                    SendMessage(_nextClipboardViewer, m.Msg, m.WParam, m.LParam);
                break;

            default:
                base.WndProc(ref m);
                break;
        }
    }

    private void OnClipboardContentChanged()
    {
        try
        {
            var contentInfo = AnalyzeClipboardContent();
            ClipboardChanged?.Invoke(this, contentInfo);
            
            _logger.LogDebug("Clipboard changed: {ContentType}, Size: {Size}", 
                contentInfo.ContentType, contentInfo.Size);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Error analyzing clipboard content");
        }
    }

    private ClipboardChangedEventArgs AnalyzeClipboardContent()
    {
        var args = new ClipboardChangedEventArgs();

        if (Clipboard.ContainsText())
        {
            var text = Clipboard.GetText();
            args.ContentType = ClipboardContentType.Text;
            args.Size = text.Length;
            args.Preview = text.Length > 200 ? text[..200] + "..." : text;
        }
        else if (Clipboard.ContainsImage())
        {
            var image = Clipboard.GetImage();
            args.ContentType = ClipboardContentType.Image;
            args.Size = image?.Width * image?.Height * 4 ?? 0; // Rough estimate
            args.Preview = $"{image?.Width}x{image?.Height}";
        }
        else if (Clipboard.ContainsFileDropList())
        {
            var files = Clipboard.GetFileDropList();
            args.ContentType = ClipboardContentType.Files;
            args.Size = files.Count;
            args.Preview = string.Join(", ", files.Cast<string>().Take(3));
        }
        else if (Clipboard.GetDataObject()?.GetFormats().Length > 0)
        {
            args.ContentType = ClipboardContentType.Other;
            var formats = Clipboard.GetDataObject()?.GetFormats();
            args.Preview = string.Join(", ", formats ?? Array.Empty<string>());
        }
        else
        {
            args.ContentType = ClipboardContentType.Empty;
        }

        return args;
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing && _nextClipboardViewer != IntPtr.Zero)
        {
            ChangeClipboardChain(Handle, _nextClipboardViewer);
            _nextClipboardViewer = IntPtr.Zero;
        }
        base.Dispose(disposing);
    }

    [DllImport("user32.dll")]
    private static extern IntPtr SetClipboardViewer(IntPtr hWndNewViewer);

    [DllImport("user32.dll")]
    private static extern bool ChangeClipboardChain(IntPtr hWndRemove, IntPtr hWndNewNext);

    [DllImport("user32.dll")]
    private static extern IntPtr SendMessage(IntPtr hWnd, int Msg, IntPtr wParam, IntPtr lParam);
}