using System.IO;
using System.Security.Cryptography;
using System.Text;
using SocialDownloader.Models;

namespace SocialDownloader.Services;

public class LicenseService
{
    private const string LicenseFile = "license.key";

    public bool IsPremium => File.Exists(LicenseFile);

    public string HardwareId
    {
        get
        {
            // Prosty HWID dla Windows: MachineGuid z rejestru + nazwa komputera
            try
            {
                using var key = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"SOFTWARE\Microsoft\Cryptography");
                var guid = key?.GetValue("MachineGuid")?.ToString() ?? string.Empty;
                var host = System.Net.Dns.GetHostName();
                return $"win:{guid}:{host}";
            }
            catch
            {
                return $"win:{System.Environment.MachineName}";
            }
        }
    }

    public void SaveLicense(string key)
    {
        File.WriteAllText(LicenseFile, key);
    }

    public void RemoveLicense()
    {
        if (File.Exists(LicenseFile))
            File.Delete(LicenseFile);
    }

    public string GenerateTestKey()
    {
        // Klucz testowy dla dewelopera (jednorazowy, wiązany z HWID)
        var hwid = HardwareId;
        var payload = $"{hwid}|premium|hd";
        var sig = Convert.ToBase64String(Encoding.UTF8.GetBytes(payload + "|signed-test"));
        return $"{{\"hwid\":\"{hwid}\",\"features\":[\"premium\",\"hd\",\"nowatermark\"],\"signature\":\"{sig}\"}}";
    }
}
