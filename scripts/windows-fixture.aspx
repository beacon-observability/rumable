<%@ Page Language="C#" AutoEventWireup="true" EnableViewState="false" %>
<%@ Import Namespace="System.IO" %>
<%@ Import Namespace="System.IO.Compression" %>
<script runat="server">
protected override void Render(System.Web.UI.HtmlTextWriter writer) {}
protected void Page_Load(object sender, EventArgs e) {
    Response.ContentType = "text/html";
    Response.Cache.SetCacheability(System.Web.HttpCacheability.NoCache);
    string leaf = Path.GetFileName(Request.Path).ToLowerInvariant();
    if (leaf == "fail.aspx") {
        string counter = Server.MapPath("~/data/fail-counter.txt");
        using (FileStream file = new FileStream(counter, FileMode.Open, FileAccess.ReadWrite, FileShare.None)) {
            int value = file.ReadByte();
            file.Position = 0; file.WriteByte(value == (byte)'0' ? (byte)'1' : (byte)'2'); file.Flush();
            if (value != (byte)'0') { Response.StatusCode = 503; Response.TrySkipIisCustomErrors = true; Response.Write("Intentional post-commit probe failure"); return; }
        }
    }
    if (leaf == "nohead.aspx") { Response.Write("<html><body>No head anchor</body></html>"); return; }
    if (leaf == "data.aspx") { Response.ContentType = "application/json"; Response.Write("{\"value\":\"</head>\"}"); return; }
    if (leaf == "download.aspx") { Response.AddHeader("Content-Disposition", "attachment; filename=fixture.html"); }
    string head = "<!doctype html><html><head><title>IIS dynamic fixture</title>";
    string tail = "</head><body>Dynamic fixture</body></html>";
    if (leaf == "chunked.aspx") {
        Response.BufferOutput = false; Response.Write(head); Response.Flush();
        System.Threading.Thread.Sleep(25); Response.Write(tail); return;
    }
    if (leaf == "gzip.aspx") {
        byte[] bytes = System.Text.Encoding.UTF8.GetBytes(head + tail);
        Response.AddHeader("Content-Encoding", "gzip");
        using (GZipStream stream = new GZipStream(Response.OutputStream, CompressionMode.Compress, true)) { stream.Write(bytes, 0, bytes.Length); }
        return;
    }
    Response.Write(head + tail);
}
</script>
