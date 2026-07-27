import { useState, useRef, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';

export default function DeviceRemotePage() {
  const { tenantID, deviceID } = useParams<{ tenantID: string; deviceID: string }>();
  const navigate = useNavigate();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [sessionID, setSessionID] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [quality, setQuality] = useState(70);
  const [fps, setFps] = useState(5);
  const [error, setError] = useState('');

  const startSession = async () => {
    if (!tenantID || !deviceID) return;
    setError('');
    try {
      const res = await fetch(`/api/v1/remote/${tenantID}/session`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ device_id: deviceID, quality, fps }),
      });
      const data = await res.json();
      if (data.session_id) {
        setSessionID(data.session_id);
        setConnected(true);
      } else {
        setError(data.error || 'Failed to start session');
      }
    } catch { setError('Connection failed'); }
  };

  const stopSession = async () => {
    if (!tenantID || !sessionID || !deviceID) return;
    try {
      await fetch(`/api/v1/remote/${tenantID}/session/${sessionID}?device_id=${deviceID}`, { method: 'DELETE' });
    } catch { /* ignore cleanup errors */ }
    setConnected(false);
    setSessionID(null);
  };

  useEffect(() => {
    if (!connected || !sessionID || !canvasRef.current) return;

    const canvas = canvasRef.current;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const wsUrl = `ws://${window.location.hostname}:8222/tenant.${tenantID}.tunnel.${sessionID}.frame`;
    let ws: WebSocket | null = null;
    let active = true;

    try {
      ws = new WebSocket(wsUrl);
      ws.onmessage = (event) => {
        if (!active || !ctx) return;
        const data = event.data instanceof Blob ? event.data : new Blob([event.data]);
        const img = new Image();
        img.onload = () => {
          canvas.width = img.width;
          canvas.height = img.height;
          ctx.drawImage(img, 0, 0);
          URL.revokeObjectURL(img.src);
        };
        img.src = URL.createObjectURL(data);
      };
      ws.onerror = () => { setError('WebSocket error'); };
      ws.onclose = () => { if (active) setConnected(false); };
    } catch { setError('WebSocket not available'); }

    return () => {
      active = false;
      if (ws) ws.close();
    };
  }, [connected, sessionID, tenantID]);

  useEffect(() => {
    return () => { if (connected) stopSession(); };
  }, []);

  const handleCanvasMouse = (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (!sessionID || !canvasRef.current) return;
    const rect = canvasRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    fetch(`/api/v1/remote/${tenantID}/session/${sessionID}/input`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ x, y, type: 'mousemove' }),
    }).catch(() => {});
  };

  return (
    <div className="min-h-screen bg-slate-950 flex flex-col">
      {/* Toolbar */}
      <div className="bg-slate-900 border-b border-slate-700 px-4 py-2 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button onClick={() => navigate(-1)} className="text-slate-400 hover:text-white text-sm">&larr; Back</button>
          <span className="text-white text-sm font-medium">Device: {deviceID?.slice(0, 12)}...</span>
          {connected && <span className="text-green-400 text-xs">● Connected</span>}
        </div>
        <div className="flex items-center gap-3">
          <label className="text-xs text-slate-400">Quality: {quality}%</label>
          <input type="range" min={10} max={100} value={quality} onChange={e => setQuality(Number(e.target.value))}
            className="w-20 h-1" />
          <label className="text-xs text-slate-400">FPS: {fps}</label>
          <input type="range" min={1} max={15} value={fps} onChange={e => setFps(Number(e.target.value))}
            className="w-16 h-1" />
          {!connected ? (
            <button onClick={startSession} className="px-3 py-1 bg-green-600 text-white text-xs rounded hover:bg-green-700">
              Connect
            </button>
          ) : (
            <button onClick={stopSession} className="px-3 py-1 bg-red-600 text-white text-xs rounded hover:bg-red-700">
              Disconnect
            </button>
          )}
        </div>
      </div>

      {error && <div className="bg-red-900/50 text-red-300 px-4 py-2 text-sm">{error}</div>}

      {/* Canvas */}
      <div className="flex-1 flex items-center justify-center p-4">
        {connected ? (
          <canvas
            ref={canvasRef}
            className="border border-slate-700 rounded shadow-2xl max-w-full max-h-full"
            style={{ cursor: 'crosshair' }}
            onMouseMove={handleCanvasMouse}
          />
        ) : (
          <div className="text-center text-slate-500">
            <p className="text-lg mb-2">Remote Control</p>
            <p className="text-sm">Click Connect to start a remote session</p>
          </div>
        )}
      </div>
    </div>
  );
}
