using System.ComponentModel;
using System.Drawing;
using System.Drawing.Imaging;
using System.Text;
using System.Windows.Forms;

namespace McpClipboard;

public static class ClipboardResources
{
    public static string GetCurrentClipboard()
    {
        try
        {
            if (Clipboard.ContainsText())
            {
                var text = Clipboard.GetText();
                return $"Text content ({text.Length} characters):\n{text}";
            }
            else if (Clipboard.ContainsImage())
            {
                var image = Clipboard.GetImage();
                using var ms = new MemoryStream();
                image.Save(ms, ImageFormat.Png);
                var base64 = Convert.ToBase64String(ms.ToArray());
                
                return $"Image content ({image.Width}x{image.Height} PNG):\ndata:image/png;base64,{base64}";
            }
            else if (Clipboard.ContainsFileDropList())
            {
                var files = Clipboard.GetFileDropList();
                var fileList = new StringBuilder();
                fileList.AppendLine($"File list ({files.Count} items):");
                
                foreach (string file in files)
                {
                    fileList.AppendLine($"- {file}");
                }
                
                return fileList.ToString();
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

    public static string GetClipboardText()
    {
        try
        {
            var text = Clipboard.ContainsText() 
                ? Clipboard.GetText()
                : "No text content in clipboard";
                
            return text;
        }
        catch (Exception ex)
        {
            return $"Error reading clipboard text: {ex.Message}";
        }
    }

    public static string GetClipboardImage()
    {
        try
        {
            if (!Clipboard.ContainsImage())
            {
                return "No image content in clipboard";
            }

            var image = Clipboard.GetImage();
            using var ms = new MemoryStream();
            image.Save(ms, ImageFormat.Png);
            var base64 = Convert.ToBase64String(ms.ToArray());
            
            return $"data:image/png;base64,{base64}";
        }
        catch (Exception ex)
        {
            return $"Error reading clipboard image: {ex.Message}";
        }
    }

    public static string GetClipboardFiles()
    {
        try
        {
            if (!Clipboard.ContainsFileDropList())
            {
                return "No files in clipboard";
            }

            var files = Clipboard.GetFileDropList();
            var result = new StringBuilder();
            result.AppendLine($"Clipboard files ({files.Count} items):");
            
            foreach (string file in files)
            {
                var info = new FileInfo(file);
                result.AppendLine($"{file}|{(info.Exists ? info.Length.ToString() : "missing")}|{(info.Exists ? info.LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss") : "")}");
            }
            
            return result.ToString();
        }
        catch (Exception ex)
        {
            return $"Error reading clipboard files: {ex.Message}";
        }
    }
}