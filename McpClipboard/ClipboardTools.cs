using System.ComponentModel;
using System.Drawing;
using System.Drawing.Imaging;
using System.Text;
using System.Windows.Forms; // .NET 9 System.Windows.Forms.Clipboard
using System.Collections.Specialized;

// NOTE: For .NET 9 Windows Desktop applications:
// - Clipboard class is System.Windows.Forms.Clipboard
// - Requires <UseWindowsForms>true</UseWindowsForms> in .csproj
// - Requires STA thread mode ([STAThread] on Main method)
// - This differs from .NET Framework which uses System.Windows.Clipboard

namespace McpClipboard;

public static class ClipboardTools
{
    private const int MaxDirectOutput = 25000;

    public static string ReadClipboard(string format = "auto", string outputPath = "", int chunkSize = MaxDirectOutput, int chunkIndex = -1)
    {
        try
        {
            // Handle file format with direct output
            if (format == "file" || !string.IsNullOrEmpty(outputPath))
            {
                return HandleFileOutput(outputPath);
            }

            if (Clipboard.ContainsText())
            {
                var text = Clipboard.GetText();
                return HandleTextContent(text, format, chunkSize, chunkIndex);
            }
            else if (Clipboard.ContainsImage())
            {
                var image = Clipboard.GetImage();
                return HandleImageContent(image, format, outputPath);
            }
            else if (Clipboard.ContainsFileDropList())
            {
                var files = Clipboard.GetFileDropList();
                return HandleFileListContent(files);
            }
            else
            {
                return "Clipboard is empty";
            }
        }
        catch (Exception ex)
        {
            return $"Error reading clipboard: {ex.Message}";
        }
    }

    public static string GetClipboardInfo()
    {
        try
        {
            var info = new StringBuilder();
            info.AppendLine("Clipboard Status:");

            if (Clipboard.ContainsText())
            {
                var text = Clipboard.GetText();
                info.AppendLine($"  Text: {text.Length} characters");
                if (text.Length > 100)
                    info.AppendLine($"  Preview: {text[..100]}...");
                else
                    info.AppendLine($"  Content: {text}");
            }

            if (Clipboard.ContainsImage())
            {
                var image = Clipboard.GetImage();
                info.AppendLine($"  Image: {image.Width}x{image.Height} pixels");
            }

            if (Clipboard.ContainsFileDropList())
            {
                var files = Clipboard.GetFileDropList();
                info.AppendLine($"  Files: {files.Count} items");
                foreach (string file in files.Cast<string>().Take(5))
                {
                    info.AppendLine($"    - {file}");
                }
                if (files.Count > 5)
                    info.AppendLine($"    ... and {files.Count - 5} more");
            }

            var dataObject = Clipboard.GetDataObject();
            if (dataObject != null)
            {
                var formats = dataObject.GetFormats();
                if (formats.Length > 0)
                {
                    info.AppendLine($"  Available formats: {string.Join(", ", formats)}");
                }
            }

            return info.ToString().TrimEnd();
        }
        catch (Exception ex)
        {
            return $"Error getting clipboard info: {ex.Message}";
        }
    }

    public static string SetClipboardText(string text)
    {
        try
        {
            Clipboard.SetText(text);
            return $"Set clipboard text ({text.Length} characters)";
        }
        catch (Exception ex)
        {
            return $"Error setting clipboard text: {ex.Message}";
        }
    }

    private static string HandleTextContent(string text, string format, int chunkSize, int chunkIndex)
    {
        switch (format)
        {
            case "base64":
                var encoded = Convert.ToBase64String(Encoding.UTF8.GetBytes(text));
                return HandleChunking(encoded, "Base64 encoded clipboard content", chunkSize, chunkIndex);

            case "text":
            case "auto":
            default:
                return HandleChunking(text, "Clipboard text content", chunkSize, chunkIndex);
        }
    }

    private static string HandleImageContent(Image image, string format, string outputPath)
    {
        if (!string.IsNullOrEmpty(outputPath))
        {
            SaveImageToFile(image, outputPath);
            return $"Image saved to: {outputPath} ({image.Width}x{image.Height})";
        }

        switch (format)
        {
            case "base64":
                using (var ms = new MemoryStream())
                {
                    image.Save(ms, ImageFormat.Png);
                    var base64 = Convert.ToBase64String(ms.ToArray());
                    
                    if (base64.Length > MaxDirectOutput)
                    {
                        var tempPath = SaveImageToTempFile(image, "png");
                        return $"Image too large for direct output ({base64.Length} bytes base64). Saved to: {tempPath}";
                    }
                    
                    return $"Clipboard image (PNG base64, {image.Width}x{image.Height}):\n{base64}";
                }

            case "file":
                var filePath = SaveImageToTempFile(image, "png");
                return $"Image saved to: {filePath} ({image.Width}x{image.Height})";

            default:
                var tempFile = SaveImageToTempFile(image, "png");
                return $"Clipboard image ({image.Width}x{image.Height} pixels). Saved to: {tempFile}";
        }
    }

    private static string HandleFileListContent(StringCollection files)
    {
        var result = new StringBuilder();
        result.AppendLine($"Clipboard contains {files.Count} files:");
        
        foreach (string file in files)
        {
            var info = new FileInfo(file);
            result.AppendLine($"  - {file} ({(info.Exists ? $"{info.Length:N0} bytes" : "not found")})");
        }

        return result.ToString().TrimEnd();
    }

    private static string HandleFileOutput(string outputPath)
    {
        if (Clipboard.ContainsImage())
        {
            var image = Clipboard.GetImage();
            var path = string.IsNullOrEmpty(outputPath) ? SaveImageToTempFile(image, "png") : outputPath;
            
            if (!string.IsNullOrEmpty(outputPath))
                SaveImageToFile(image, outputPath);
                
            return $"Image saved to: {path} ({image.Width}x{image.Height})";
        }
        else if (Clipboard.ContainsText())
        {
            var text = Clipboard.GetText();
            var path = string.IsNullOrEmpty(outputPath) ? SaveTextToTempFile(text) : outputPath;
            
            if (!string.IsNullOrEmpty(outputPath))
                File.WriteAllText(outputPath, text);
                
            return $"Text saved to: {path} ({text.Length} bytes)";
        }
        else
        {
            return "No suitable clipboard content for file output";
        }
    }

    private static string HandleChunking(string content, string description, int chunkSize, int chunkIndex)
    {
        if (content.Length <= chunkSize)
        {
            return $"{description}:\n{content}";
        }

        var totalChunks = (content.Length + chunkSize - 1) / chunkSize;

        if (chunkIndex < 0)
        {
            return $"{description} too large ({content.Length:N0} bytes). " +
                   $"Available in {totalChunks} chunks of {chunkSize:N0} bytes each. " +
                   $"Use chunkIndex parameter (0-{totalChunks - 1}) to get specific chunks.";
        }

        if (chunkIndex >= totalChunks)
        {
            return $"Chunk index {chunkIndex} out of range (0-{totalChunks - 1})";
        }

        var start = chunkIndex * chunkSize;
        var end = Math.Min(start + chunkSize, content.Length);
        var chunk = content[start..end];

        return $"Chunk {chunkIndex + 1}/{totalChunks} ({start}-{end - 1} of {content.Length:N0} bytes):\n{chunk}";
    }

    private static void SaveImageToFile(Image image, string filePath)
    {
        var extension = Path.GetExtension(filePath).ToLowerInvariant();
        var format = extension switch
        {
            ".jpg" or ".jpeg" => ImageFormat.Jpeg,
            ".gif" => ImageFormat.Gif,
            ".bmp" => ImageFormat.Bmp,
            ".tiff" => ImageFormat.Tiff,
            _ => ImageFormat.Png
        };

        image.Save(filePath, format);
    }

    private static string SaveImageToTempFile(Image image, string extension)
    {
        var tempPath = Path.GetTempFileName();
        var finalPath = Path.ChangeExtension(tempPath, extension);
        
        File.Delete(tempPath); // Remove the temp file created by GetTempFileName
        SaveImageToFile(image, finalPath);
        
        return finalPath;
    }

    private static string SaveTextToTempFile(string text)
    {
        var tempPath = Path.GetTempFileName();
        var finalPath = Path.ChangeExtension(tempPath, "txt");
        
        File.Delete(tempPath);
        File.WriteAllText(finalPath, text);
        
        return finalPath;
    }
}