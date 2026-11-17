#include <windows.h>
#include <mmdeviceapi.h>
#include <tlhelp32.h>
#include <map>
#include <audioclient.h>
#include <audiopolicy.h>
#include <propidl.h>
#include <wrl.h>
#include <vector>
#include <string>
#include <set>
#include <algorithm>
#include <cmath>
using Microsoft::WRL::ComPtr;

// The installed SDK predates process loopback; these are its documented ABI values.
struct ProcessLoopbackParams { int type; DWORD pid; int mode; };
struct AppAudioInfo { DWORD pid; wchar_t path[1024]; };
class AppActivation : public Microsoft::WRL::RuntimeClass<Microsoft::WRL::RuntimeClassFlags<Microsoft::WRL::ClassicCom>, Microsoft::WRL::FtmBase, IActivateAudioInterfaceCompletionHandler> {
public:
 HANDLE event = CreateEventW(nullptr, TRUE, FALSE, nullptr);
 HRESULT result = E_PENDING;
 ComPtr<IAudioClient> client;
 ~AppActivation(){ if(event) CloseHandle(event); }
 HRESULT STDMETHODCALLTYPE ActivateCompleted(IActivateAudioInterfaceAsyncOperation* op) override {
  ComPtr<IUnknown> object; HRESULT activation = E_FAIL;
  result = op->GetActivateResult(&activation, &object);
  if(SUCCEEDED(result)) result = activation;
  if(SUCCEEDED(result)) result = object.As(&client);
  SetEvent(event); return S_OK;
 }
};
struct AppCapture {
 ComPtr<IAudioClient> client;
 ComPtr<IAudioCaptureClient> capture;
 HANDLE event = CreateEventW(nullptr, FALSE, FALSE, nullptr);
 ~AppCapture(){if(client) client->Stop();if(event) CloseHandle(event);}
};
extern "C" __declspec(dllexport) HRESULT ql_app_list(AppAudioInfo* entries, UINT capacity, UINT* count) {
 if(!entries || !count || capacity>256) return E_INVALIDARG;
 *count=0;
 HRESULT apartment=CoInitializeEx(nullptr,COINIT_MULTITHREADED);
 if(FAILED(apartment) && apartment!=RPC_E_CHANGED_MODE) return apartment;
 HRESULT result=[&]()->HRESULT {
  ComPtr<IMMDeviceEnumerator> enumerator; HRESULT hr=CoCreateInstance(__uuidof(MMDeviceEnumerator),nullptr,CLSCTX_ALL,IID_PPV_ARGS(&enumerator));if(FAILED(hr))return hr;
  ComPtr<IMMDeviceCollection> devices;hr=enumerator->EnumAudioEndpoints(eRender,DEVICE_STATE_ACTIVE,&devices);if(FAILED(hr))return hr;
  UINT deviceCount=0;devices->GetCount(&deviceCount);std::set<DWORD> seen;
  std::map<DWORD,DWORD> parents;HANDLE snapshot=CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS,0);
  if(snapshot!=INVALID_HANDLE_VALUE){PROCESSENTRY32W process={};process.dwSize=sizeof(process);if(Process32FirstW(snapshot,&process)){do{parents[process.th32ProcessID]=process.th32ParentProcessID;}while(Process32NextW(snapshot,&process));}CloseHandle(snapshot);}
  for(UINT d=0;d<deviceCount;d++){
   ComPtr<IMMDevice> device;if(FAILED(devices->Item(d,&device)))continue;
   ComPtr<IAudioSessionManager2> manager;if(FAILED(device->Activate(__uuidof(IAudioSessionManager2),CLSCTX_ALL,nullptr,&manager)))continue;
   ComPtr<IAudioSessionEnumerator> sessions;if(FAILED(manager->GetSessionEnumerator(&sessions)))continue;
   int n=0;sessions->GetCount(&n);
   for(int i=0;i<n && *count<capacity;i++){
    ComPtr<IAudioSessionControl> session;if(FAILED(sessions->GetSession(i,&session)))continue;
    AudioSessionState state;if(FAILED(session->GetState(&state)) || state!=AudioSessionStateActive)continue;
    ComPtr<IAudioSessionControl2> info;if(FAILED(session.As(&info)))continue;
    DWORD pid=0;if(FAILED(info->GetProcessId(&pid)) || !pid || pid==GetCurrentProcessId() || !seen.insert(pid).second)continue;
    HANDLE process=OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION,FALSE,pid);if(!process)continue;
    AppAudioInfo entry={};entry.pid=pid;DWORD size=1024;
    BOOL ok=QueryFullProcessImageNameW(process,0,entry.path,&size);CloseHandle(process);
    if(ok && size<1024){
     // Capture one process tree per app so browser child sessions are not mixed twice.
     DWORD root=pid;
     for(int depth=0;depth<32;depth++){
      auto parent=parents.find(root);if(parent==parents.end() || !parent->second || parent->second==root || parent->second==GetCurrentProcessId())break;
      HANDLE process=OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION,FALSE,parent->second);if(!process)break;
      wchar_t parentPath[1024]={};DWORD length=1024;BOOL found=QueryFullProcessImageNameW(process,0,parentPath,&length);CloseHandle(process);
      if(!found || _wcsicmp(parentPath,entry.path)!=0)break;root=parent->second;
     }
     entry.pid=root;bool duplicate=false;for(UINT k=0;k<*count;k++)if(entries[k].pid==root)duplicate=true;
     if(!duplicate && root!=GetCurrentProcessId())entries[(*count)++]=entry;
    }
   }
  }
  return S_OK;
 }();
 if(SUCCEEDED(apartment))CoUninitialize();return result;
}
extern "C" __declspec(dllexport) HRESULT ql_app_open(DWORD pid, AppCapture** output) {
 if(!pid || pid==GetCurrentProcessId() || !output)return E_INVALIDARG;
 *output=nullptr;
 auto handler=Microsoft::WRL::Make<AppActivation>();if(!handler || !handler->event)return E_OUTOFMEMORY;
 ProcessLoopbackParams params={1,pid,0};PROPVARIANT variant={};variant.vt=VT_BLOB;variant.blob.cbSize=sizeof(params);variant.blob.pBlobData=reinterpret_cast<BYTE*>(&params);
 ComPtr<IActivateAudioInterfaceAsyncOperation> operation;
 HRESULT hr=ActivateAudioInterfaceAsync(L"VAD\\Process_Loopback",__uuidof(IAudioClient),&variant,handler.Get(),&operation);if(FAILED(hr))return hr;
 DWORD wait=WaitForSingleObject(handler->event,5000);if(wait!=WAIT_OBJECT_0)return HRESULT_FROM_WIN32(wait==WAIT_TIMEOUT?ERROR_TIMEOUT:GetLastError());
 if(FAILED(handler->result))return handler->result;
 auto capture=new(std::nothrow) AppCapture();if(!capture)return E_OUTOFMEMORY;
 capture->client=handler->client;
 WAVEFORMATEX format={};format.wFormatTag=WAVE_FORMAT_IEEE_FLOAT;format.nChannels=2;format.nSamplesPerSec=48000;format.wBitsPerSample=32;format.nBlockAlign=8;format.nAvgBytesPerSec=384000;
 hr=capture->client->Initialize(AUDCLNT_SHAREMODE_SHARED,AUDCLNT_STREAMFLAGS_LOOPBACK|AUDCLNT_STREAMFLAGS_EVENTCALLBACK|AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM,0,0,&format,nullptr);
 if(SUCCEEDED(hr))hr=capture->client->GetService(IID_PPV_ARGS(&capture->capture));
 if(SUCCEEDED(hr) && !capture->event)hr=E_OUTOFMEMORY;
 if(SUCCEEDED(hr))hr=capture->client->SetEventHandle(capture->event);
 if(SUCCEEDED(hr))hr=capture->client->Start();
 if(FAILED(hr)){delete capture;return hr;}*output=capture;return S_OK;
}
extern "C" __declspec(dllexport) HRESULT ql_app_read(AppCapture* capture,float* output,UINT capacity,UINT* count){
 if(!capture || !output || !count || capacity>8192)return E_INVALIDARG;
 *count=0;
 for(int packet=0;packet<32;packet++){
  UINT frames=0;HRESULT hr=capture->capture->GetNextPacketSize(&frames);if(FAILED(hr))return hr;
  if(!frames || frames>capacity-*count)return S_OK;
  BYTE* data=nullptr;DWORD flags=0;hr=capture->capture->GetBuffer(&data,&frames,&flags,nullptr,nullptr);if(FAILED(hr))return hr;
  for(UINT i=0;i<frames;i++){
   float value=0;if(!(flags&AUDCLNT_BUFFERFLAGS_SILENT) && data){auto samples=reinterpret_cast<float*>(data);value=(samples[i*2]+samples[i*2+1])*.5f;}
   output[(*count)++]=std::isfinite(value)?value:0;
  }
  hr=capture->capture->ReleaseBuffer(frames);if(FAILED(hr))return hr;
 }
 return S_OK;
}
extern "C" __declspec(dllexport) void ql_app_close(AppCapture* capture){delete capture;}
