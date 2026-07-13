using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.DependencyInjection;
using ModelContextProtocol;

namespace McpClipboard;

class Program
{
    // CRITICAL: [STAThread] required for System.Windows.Forms.Clipboard access
    // Without this, clipboard operations will fail or hang due to COM apartment mismatch
    [STAThread]
    static async Task Main(string[] args)
    {
        var builder = Host.CreateApplicationBuilder(args);
        
        // Add clipboard service first
        builder.Services.AddSingleton<ClipboardService>();
        
        // Configure logging
        builder.Logging.AddConsole();
        builder.Logging.SetMinimumLevel(LogLevel.Information);
        
        var host = builder.Build();
        
        var logger = host.Services.GetRequiredService<ILogger<Program>>();
        logger.LogInformation("Starting MCP Clipboard Service...");
        
        // Start clipboard monitoring in background
        var clipboardService = host.Services.GetRequiredService<ClipboardService>();
        clipboardService.ClipboardChanged += (sender, e) =>
        {
            logger.LogInformation("Clipboard changed: {ContentType}, Size: {Size}", 
                e.ContentType, e.Size);
        };
        
        // Start monitoring without blocking
        _ = Task.Run(async () =>
        {
            try
            {
                await clipboardService.StartMonitoringAsync();
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Failed to start clipboard monitoring");
            }
        });
        
        logger.LogInformation("MCP Clipboard Service ready");
        logger.LogInformation("Available functions:");
        logger.LogInformation("- ClipboardTools.ReadClipboard()");
        logger.LogInformation("- ClipboardTools.GetClipboardInfo()");
        logger.LogInformation("- ClipboardTools.SetClipboardText()");
        
        // Simple test
        try
        {
            var info = ClipboardTools.GetClipboardInfo();
            logger.LogInformation("Current clipboard: {Info}", info);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to read clipboard");
        }
        
        // Keep running
        logger.LogInformation("Press Ctrl+C to exit...");
        await host.RunAsync();
    }
}
