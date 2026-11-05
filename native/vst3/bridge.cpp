// Quiet Lab VST3 host. Steinberg interfaces are used under their MIT license.
#define NOMINMAX
#define INIT_CLASS_IID
#include <windows.h>
#include <objbase.h>
#include "pluginterfaces/base/ipluginbase.h"
#include "pluginterfaces/base/ibstream.h"
#include "pluginterfaces/vst/ivstaudioprocessor.h"
#include "pluginterfaces/vst/ivsteditcontroller.h"
#include "pluginterfaces/vst/ivsthostapplication.h"
#include "pluginterfaces/vst/ivstparameterchanges.h"
#include "pluginterfaces/vst/ivstprocesscontext.h"
#include "pluginterfaces/vst/vstspeaker.h"
#include "pluginterfaces/gui/iplugview.h"
#include <algorithm>
#include <atomic>
#include <cmath>
#include <cstring>
#include <functional>
#include <future>
#include <map>
#include <memory>
#include <mutex>
#include <sstream>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

using namespace Steinberg;
using namespace Steinberg::Vst;
constexpr int blockSize=128;
constexpr int stateLimit=16*1024*1024;
#define REFCOUNT std::atomic<uint32> refs{1}; uint32 PLUGIN_API addRef() override {return ++refs;} uint32 PLUGIN_API release() override {auto r=--refs;if(!r)delete this;return r;}
#define FIXED_REF uint32 PLUGIN_API addRef() override{return 1;} uint32 PLUGIN_API release() override{return 1;}
template<class T> bool iidIs(const TUID id){return FUnknownPrivate::iidEqual(id,T::iid);}
template<class T> T* query(FUnknown* o){T* p=nullptr;if(o)o->queryInterface(T::iid,(void**)&p);return p;}
template<class T> void release(T*& p){if(p){p->release();p=nullptr;}}
std::wstring wide(const std::string& s){int n=MultiByteToWideChar(CP_UTF8,0,s.c_str(),-1,nullptr,0);std::wstring w(n,0);MultiByteToWideChar(CP_UTF8,0,s.c_str(),-1,&w[0],n);if(!w.empty())w.pop_back();return w;}
std::string utf8(const TChar* s){int n=WideCharToMultiByte(CP_UTF8,0,(const wchar_t*)s,-1,nullptr,0,nullptr,nullptr);std::string r(n,0);WideCharToMultiByte(CP_UTF8,0,(const wchar_t*)s,-1,&r[0],n,nullptr,nullptr);if(!r.empty())r.pop_back();return r;}
std::string quote(const std::string& s){std::string r="\"";for(unsigned char c:s){if(c=='"'||c=='\\'){r+='\\';r+=c;}else if(c<32)r+=' ';else r+=c;}return r+'"';}
void copyText(char* out,int cap,const std::string& s){if(out&&cap>0){auto n=std::min<size_t>(cap-1,s.size());memcpy(out,s.data(),n);out[n]=0;}}

class Stream:public IBStream {
public:
 std::vector<unsigned char> bytes;int64 pos=0;REFCOUNT
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IBStream>(id)){*o=(IBStream*)this;addRef();return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API read(void* b,int32 n,int32* done) override{if(n<0)return kInvalidArgument;auto count=(int32)std::min<int64>(n,std::max<int64>(0,(int64)bytes.size()-pos));if(count)memcpy(b,bytes.data()+pos,count);pos+=count;if(done)*done=count;return count==n?kResultOk:kResultFalse;}
 tresult PLUGIN_API write(void* b,int32 n,int32* done) override{if(n<0||pos+n>stateLimit)return kOutOfMemory;bytes.resize((size_t)std::max<int64>(bytes.size(),pos+n));if(n)memcpy(bytes.data()+pos,b,n);pos+=n;if(done)*done=n;return kResultOk;}
 tresult PLUGIN_API seek(int64 p,int32 mode,int64* result) override{int64 next=p+(mode==kIBSeekCur?pos:mode==kIBSeekEnd?(int64)bytes.size():0);if(next<0||next>stateLimit)return kInvalidArgument;pos=next;if(result)*result=pos;return kResultOk;}
 tresult PLUGIN_API tell(int64* p) override{if(!p)return kInvalidArgument;*p=pos;return kResultOk;}
};
class Attributes:public IAttributeList {
 struct Value{int64 integer=0;double number=0;std::basic_string<TChar> text;std::vector<unsigned char> binary;};
 std::map<std::string,Value> values;
public:REFCOUNT
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IAttributeList>(id)){*o=(IAttributeList*)this;addRef();return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API setInt(AttrID id,int64 v) override{values[id].integer=v;return kResultOk;}
 tresult PLUGIN_API getInt(AttrID id,int64& v) override{auto it=values.find(id);if(it==values.end())return kResultFalse;v=it->second.integer;return kResultOk;}
 tresult PLUGIN_API setFloat(AttrID id,double v) override{values[id].number=v;return kResultOk;}
 tresult PLUGIN_API getFloat(AttrID id,double& v) override{auto it=values.find(id);if(it==values.end())return kResultFalse;v=it->second.number;return kResultOk;}
 tresult PLUGIN_API setString(AttrID id,const TChar* s) override{values[id].text=s;return kResultOk;}
 tresult PLUGIN_API getString(AttrID id,TChar* s,uint32 size) override{auto it=values.find(id);if(it==values.end()||size<2)return kResultFalse;auto n=std::min<size_t>(size/2-1,it->second.text.size());memcpy(s,it->second.text.data(),n*2);s[n]=0;return kResultOk;}
 tresult PLUGIN_API setBinary(AttrID id,const void* p,uint32 size) override{if(size>stateLimit)return kOutOfMemory;auto b=(const unsigned char*)p;values[id].binary.assign(b,b+size);return kResultOk;}
 tresult PLUGIN_API getBinary(AttrID id,const void*& p,uint32& size) override{auto it=values.find(id);if(it==values.end())return kResultFalse;p=it->second.binary.data();size=(uint32)it->second.binary.size();return kResultOk;}
};
class Message:public IMessage {
 std::string name;Attributes* attributes=new Attributes;
public:REFCOUNT
 ~Message(){attributes->release();}
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IMessage>(id)){*o=(IMessage*)this;addRef();return kResultOk;}return kNoInterface;}
 FIDString PLUGIN_API getMessageID() override{return name.c_str();}
 void PLUGIN_API setMessageID(FIDString id) override{name=id?id:"";}
 IAttributeList* PLUGIN_API getAttributes() override{return attributes;}
};
class Host:public IHostApplication {
public:FIXED_REF
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IHostApplication>(id)){*o=(IHostApplication*)this;return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API getName(String128 name) override{const wchar_t* n=L"Quiet Lab";memcpy(name,n,(wcslen(n)+1)*2);return kResultOk;}
 tresult PLUGIN_API createInstance(TUID cid,TUID id,void** o) override{*o=nullptr;FUnknown* object=nullptr;if(iidIs<IMessage>(cid))object=new Message;else if(iidIs<IAttributeList>(cid))object=new Attributes;if(!object)return kNoInterface;auto r=object->queryInterface(id,o);object->release();return r;}
} host;

DWORD uiThreadID=0;std::once_flag uiStarted;
void startUI(){std::call_once(uiStarted,[]{std::promise<void> ready;auto future=ready.get_future();std::thread([&ready]{CoInitializeEx(nullptr,COINIT_APARTMENTTHREADED);MSG msg;PeekMessageW(&msg,nullptr,0,0,PM_NOREMOVE);uiThreadID=GetCurrentThreadId();ready.set_value();while(GetMessageW(&msg,nullptr,0,0)>0){if(msg.message==WM_APP+44){auto task=(std::function<void()>*)msg.lParam;(*task)();}else{TranslateMessage(&msg);DispatchMessageW(&msg);}}CoUninitialize();}).detach();future.get();});}
template<class F> auto onUI(F fn)->decltype(fn()){startUI();if(GetCurrentThreadId()==uiThreadID)return fn();std::packaged_task<decltype(fn())()> task(fn);auto result=task.get_future();std::function<void()> call=[&]{task();};if(!PostThreadMessageW(uiThreadID,WM_APP+44,0,(LPARAM)&call))throw std::runtime_error("VST UI thread unavailable");return result.get();}

struct Module {
 HMODULE dll=nullptr;IPluginFactory* factory=nullptr;
 explicit Module(const std::string& path){
  auto binary=wide(path);if(GetFileAttributesW(binary.c_str())&FILE_ATTRIBUTE_DIRECTORY){auto at=binary.find_last_of(L"\\/");auto name=binary.substr(at==std::wstring::npos?0:at+1);binary+=L"\\Contents\\x86_64-win\\"+name;}
  dll=LoadLibraryExW(binary.c_str(),nullptr,LOAD_WITH_ALTERED_SEARCH_PATH);if(!dll)throw std::runtime_error("Cannot load 64-bit VST3 module (missing dependency or incompatible architecture)");
  auto init=(bool(*)())GetProcAddress(dll,"InitDll");if(init&&!init()){FreeLibrary(dll);dll=nullptr;throw std::runtime_error("Plugin initialization failed");}
  auto get=(IPluginFactory*(*)())GetProcAddress(dll,"GetPluginFactory");if(get)factory=get();if(!factory){FreeLibrary(dll);dll=nullptr;throw std::runtime_error("Not a VST3 plugin");}
  auto f3=query<IPluginFactory3>(factory);if(f3){f3->setHostContext(&host);f3->release();}
 }
 ~Module(){release(factory);if(dll){auto exit=(bool(*)())GetProcAddress(dll,"ExitDll");if(exit)exit();FreeLibrary(dll);}}
};
std::string classHex(const char* id){static const char* h="0123456789abcdef";std::string s;for(int i=0;i<16;i++){unsigned char c=id[i];s+=h[c>>4];s+=h[c&15];}return s;}
std::shared_ptr<Module> sharedModule(const std::string& path){static std::map<std::string,std::weak_ptr<Module>> modules;auto m=modules[path].lock();if(!m){m=std::make_shared<Module>(path);modules[path]=m;}return m;}
bool parseClass(const std::string& s,TUID id){if(s.size()!=32)return false;for(int i=0;i<16;i++){auto hex=[](char c){return c>='0'&&c<='9'?c-'0':c>='a'&&c<='f'?c-'a'+10:c>='A'&&c<='F'?c-'A'+10:-1;};int a=hex(s[i*2]),b=hex(s[i*2+1]);if(a<0||b<0)return false;id[i]=(char)(a*16+b);}return true;}

struct Queue:public IParamValueQueue {
 ParamID id=0;double value=0;FIXED_REF
 tresult PLUGIN_API queryInterface(const TUID iid,void** o) override{*o=nullptr;if(iidIs<FUnknown>(iid)||iidIs<IParamValueQueue>(iid)){*o=(IParamValueQueue*)this;return kResultOk;}return kNoInterface;}
 ParamID PLUGIN_API getParameterId() override{return id;}
 int32 PLUGIN_API getPointCount() override{return 1;}
 tresult PLUGIN_API getPoint(int32 i,int32& offset,ParamValue& v) override{if(i)return kResultFalse;offset=0;v=value;return kResultOk;}
 tresult PLUGIN_API addPoint(int32,ParamValue,int32&) override{return kNotImplemented;}
};
struct Changes:public IParameterChanges {
 std::unique_ptr<Queue[]> queues;std::vector<int> changed;int count=0;FIXED_REF
 void setup(int n){queues.reset(new Queue[n]);changed.resize(n);}
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IParameterChanges>(id)){*o=(IParameterChanges*)this;return kResultOk;}return kNoInterface;}
 int32 PLUGIN_API getParameterCount() override{return count;}
 IParamValueQueue* PLUGIN_API getParameterData(int32 i) override{return i>=0&&i<count?&queues[changed[i]]:nullptr;}
 IParamValueQueue* PLUGIN_API addParameterData(const ParamID&,int32&) override{return nullptr;}
};
struct Group;
struct Handler:public IComponentHandler,public IComponentHandler2 {
 Group* group;explicit Handler(Group* g):group(g){} FIXED_REF
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IComponentHandler>(id)){*o=(IComponentHandler*)this;return kResultOk;}if(iidIs<IComponentHandler2>(id)){*o=(IComponentHandler2*)this;return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API beginEdit(ParamID) override{return kResultOk;}
 tresult PLUGIN_API endEdit(ParamID) override{return kResultOk;}
 tresult PLUGIN_API performEdit(ParamID id,ParamValue v) override;
 tresult PLUGIN_API restartComponent(int32 flags) override;
 tresult PLUGIN_API setDirty(TBool) override{return kResultOk;}
 tresult PLUGIN_API requestOpenEditor(FIDString) override{return kNotImplemented;}
 tresult PLUGIN_API startGroupEdit() override{return kResultOk;}
 tresult PLUGIN_API finishGroupEdit() override{return kResultOk;}
};
struct Instance {
 IComponent* component=nullptr;IAudioProcessor* processor=nullptr;IEditController* controller=nullptr;bool separate=false,initialized=false,active=false,processing=false;
 IConnectionPoint* componentConnection=nullptr;IConnectionPoint* controllerConnection=nullptr;
 Changes changes;std::vector<double> previous;
 float input[2][blockSize]{},output[2][blockSize]{};float* inPtrs[2]{input[0],input[1]};float* outPtrs[2]{output[0],output[1]};
 AudioBusBuffers inBus,outBus;ProcessContext context{};int inputChannels=0,outputChannels=0;int64 frames=0;
 ~Instance(){if(processing)processor->setProcessing(false);if(active)component->setActive(false);if(componentConnection&&controllerConnection){componentConnection->disconnect(controllerConnection);controllerConnection->disconnect(componentConnection);}release(componentConnection);release(controllerConnection);if(controller){controller->setComponentHandler(nullptr);if(separate)controller->terminate();}release(controller);release(processor);if(component&&initialized)component->terminate();release(component);}
};

struct Frame:public IPlugFrame {
 HWND window=nullptr;FIXED_REF
 tresult PLUGIN_API queryInterface(const TUID id,void** o) override{*o=nullptr;if(iidIs<FUnknown>(id)||iidIs<IPlugFrame>(id)){*o=(IPlugFrame*)this;return kResultOk;}return kNoInterface;}
 tresult PLUGIN_API resizeView(IPlugView* view,ViewRect* size) override{if(!window||!size)return kResultFalse;RECT r{0,0,size->getWidth(),size->getHeight()};AdjustWindowRect(&r,(DWORD)GetWindowLongPtrW(window,GWL_STYLE),FALSE);SetWindowPos(window,nullptr,0,0,r.right-r.left,r.bottom-r.top,SWP_NOMOVE|SWP_NOZORDER);view->onSize(size);return kResultOk;}
};
struct Group {
 std::shared_ptr<Module> module;std::vector<std::unique_ptr<Instance>> voices;std::vector<ParameterInfo> parameters;std::unique_ptr<std::atomic<double>[]> values;
 Handler handler{this};Frame frame;IPlugView* view=nullptr;std::vector<unsigned char> cachedState;std::string name;std::atomic<bool> needsRestart{false};
 ~Group(){closeEditor();voices.clear();module.reset();}
 void closeEditor(){if(view){view->removed();view->setFrame(nullptr);view->release();view=nullptr;}if(frame.window){auto w=frame.window;frame.window=nullptr;DestroyWindow(w);}}
 void update(ParamID id,double value){if(!std::isfinite(value))return;for(size_t i=0;i<parameters.size();i++)if(parameters[i].id==id){values[i].store(std::max(0.,std::min(1.,value)),std::memory_order_relaxed);break;}}
 void refresh(){if(voices.empty()||!voices[0]->controller)return;auto c=voices[0]->controller;for(size_t i=0;i<parameters.size();i++)values[i].store(c->getParamNormalized(parameters[i].id),std::memory_order_relaxed);}
};
tresult Handler::performEdit(ParamID id,ParamValue v){group->update(id,v);return kResultOk;}
tresult Handler::restartComponent(int32 flags){if(flags&kParamValuesChanged)group->refresh();if(flags&(kIoChanged|kReloadComponent|kLatencyChanged)){group->needsRestart=true;return kNotImplemented;}return kResultOk;}

void restoreState(Instance& i,const unsigned char* data,int length){
 if(!data||length<4)return;uint32 n=0;memcpy(&n,data,4);if(n>(uint32)length-4)throw std::runtime_error("Invalid plugin state");
 Stream stream;stream.bytes.assign(data+4,data+4+n);if(i.component->setState(&stream)!=kResultOk&&n)throw std::runtime_error("Plugin could not restore its saved state");stream.pos=0;if(i.controller)i.controller->setComponentState(&stream);
 if(i.controller&&(int)n+4<length){Stream cs;cs.bytes.assign(data+4+n,data+length);i.controller->setState(&cs);}
}
std::unique_ptr<Instance> createInstance(Group& g,const TUID cid,const unsigned char* state,int stateSize,bool primary){
 auto p=std::make_unique<Instance>();
 if(g.module->factory->createInstance(cid,IComponent::iid,(void**)&p->component)!=kResultOk||!p->component)throw std::runtime_error("Cannot create plugin processor");
 p->component->setIoMode(kSimple);
 if(p->component->initialize(&host)!=kResultOk)throw std::runtime_error("Plugin processor initialization failed");p->initialized=true;
 p->processor=query<IAudioProcessor>(p->component);if(!p->processor||p->processor->canProcessSampleSize(kSample32)!=kResultOk)throw std::runtime_error("Plugin does not support 32-bit audio");
 p->controller=query<IEditController>(p->component);
 if(!p->controller){TUID controllerID;if(p->component->getControllerClassId(controllerID)==kResultOk&&g.module->factory->createInstance(controllerID,IEditController::iid,(void**)&p->controller)==kResultOk&&p->controller){p->separate=true;if(p->controller->initialize(&host)!=kResultOk)throw std::runtime_error("Plugin controller initialization failed");}}
 if(p->controller){if(primary)p->controller->setComponentHandler(&g.handler);p->componentConnection=query<IConnectionPoint>(p->component);p->controllerConnection=query<IConnectionPoint>(p->controller);if(p->separate&&p->componentConnection&&p->controllerConnection){p->componentConnection->connect(p->controllerConnection);p->controllerConnection->connect(p->componentConnection);}}
 int ins=p->component->getBusCount(kAudio,kInput),outs=p->component->getBusCount(kAudio,kOutput);
 if(ins<1||outs<1)throw std::runtime_error("Choose an audio effect with an audio input and output; instruments are unsupported");
 // Main bus only. Extra audio and all MIDI buses remain inactive.
 for(int d=0;d<2;d++){auto direction=d==0?kInput:kOutput;int n=p->component->getBusCount(kAudio,direction);for(int j=0;j<n;j++)p->component->activateBus(kAudio,direction,j,j==0);int events=p->component->getBusCount(kEvent,direction);for(int j=0;j<events;j++)p->component->activateBus(kEvent,direction,j,false);}
 SpeakerArrangement stereo=SpeakerArr::kStereo;
 if(ins==1&&outs==1)p->processor->setBusArrangements(&stereo,1,&stereo,1);
 BusInfo input{},output{};p->component->getBusInfo(kAudio,kInput,0,input);p->component->getBusInfo(kAudio,kOutput,0,output);
 if(input.channelCount<1||input.channelCount>2||output.channelCount<1||output.channelCount>2||ins!=1||outs!=1)throw std::runtime_error("This host currently supports one mono/stereo input bus and one mono/stereo output bus");
 p->inputChannels=input.channelCount;p->outputChannels=output.channelCount;
 if(stateSize)restoreState(*p,state,stateSize);
 else if(p->controller){Stream stream;if(p->component->getState(&stream)==kResultOk){stream.pos=0;p->controller->setComponentState(&stream);}}
 ProcessSetup setup{kRealtime,kSample32,blockSize,48000.};if(p->processor->setupProcessing(setup)!=kResultOk)throw std::runtime_error("Plugin rejected the audio format");
 if(p->component->setActive(true)!=kResultOk)throw std::runtime_error("Cannot activate plugin");p->active=true;
 if(p->processor->setProcessing(true)!=kResultOk)throw std::runtime_error("Cannot start plugin processing");p->processing=true;
 p->inBus.numChannels=p->inputChannels;p->inBus.channelBuffers32=p->inPtrs;p->outBus.numChannels=p->outputChannels;p->outBus.channelBuffers32=p->outPtrs;
 p->context.sampleRate=48000.;p->context.tempo=120.;p->context.timeSigNumerator=4;p->context.timeSigDenominator=4;p->context.state=ProcessContext::kPlaying|ProcessContext::kTempoValid|ProcessContext::kTimeSigValid;
 return p;
}
LRESULT CALLBACK editorProc(HWND w,UINT msg,WPARAM wp,LPARAM lp){auto g=(Group*)GetWindowLongPtrW(w,GWLP_USERDATA);if(msg==WM_NCCREATE){g=(Group*)((CREATESTRUCTW*)lp)->lpCreateParams;SetWindowLongPtrW(w,GWLP_USERDATA,(LONG_PTR)g);}if(msg==WM_CLOSE&&g){g->closeEditor();return 0;}return DefWindowProcW(w,msg,wp,lp);}

#define EXPORT extern "C" __declspec(dllexport)
EXPORT int ql_scan(const char* path,char* out,int cap){try{auto json=onUI([&]{Module m(path);PFactoryInfo vendor{};m.factory->getFactoryInfo(&vendor);auto f2=query<IPluginFactory2>(m.factory);std::string r="[";bool first=true;for(int i=0;i<m.factory->countClasses();i++){PClassInfo info{};if(m.factory->getClassInfo(i,&info)!=kResultOk||strcmp(info.category,kVstAudioEffectClass))continue;std::string sub;PClassInfo2 info2{};if(f2&&f2->getClassInfo2(i,&info2)==kResultOk)sub=info2.subCategories;if(sub.find("Instrument")!=std::string::npos)continue;if(!first)r+=",";first=false;r+="{\"ClassID\":"+quote(classHex(info.cid))+",\"Name\":"+quote(info.name)+",\"Vendor\":"+quote(vendor.vendor)+",\"Category\":"+quote(sub)+"}";}release(f2);return r+"]";});copyText(out,cap,json);return 1;}catch(const std::exception& e){copyText(out,cap,e.what());return 0;}}
EXPORT Group* ql_open(const char* path,const char* classID,const unsigned char* state,int stateSize,int voices,char* error,int cap){try{return onUI([&]{if(voices<1||voices>8)throw std::runtime_error("Invalid output count");TUID cid;if(!parseClass(classID,cid))throw std::runtime_error("Invalid VST3 class ID");auto g=std::make_unique<Group>();g->module=sharedModule(path);auto first=createInstance(*g,cid,state,stateSize,true);auto c=first->controller;if(c){int n=c->getParameterCount();if(n<0||n>16384)throw std::runtime_error("Unsupported plugin parameter count");for(int i=0;i<n;i++){ParameterInfo info{};if(c->getParameterInfo(i,info)==kResultOk)g->parameters.push_back(info);}}g->values.reset(new std::atomic<double>[g->parameters.size()]);for(size_t i=0;i<g->parameters.size();i++)g->values[i].store(c->getParamNormalized(g->parameters[i].id));g->voices.push_back(std::move(first));for(int i=1;i<voices;i++)g->voices.push_back(createInstance(*g,cid,state,stateSize,false));for(auto& instance:g->voices){instance->changes.setup((int)g->parameters.size());instance->previous.assign(g->parameters.size(),-1);for(size_t i=0;i<g->parameters.size();i++)instance->changes.queues[i].id=g->parameters[i].id;}return g.release();});}catch(const std::exception& e){copyText(error,cap,e.what());return nullptr;}}
EXPORT int ql_process(Group* g,int voice,const float* input,float* output,int n){if(!g||voice<0||voice>=(int)g->voices.size()||n!=blockSize)return 0;auto& p=*g->voices[voice];for(int c=0;c<p.inputChannels;c++)memcpy(p.input[c],input,n*sizeof(float));for(auto& channel:p.output)memset(channel,0,sizeof(channel));p.changes.count=0;for(size_t i=0;i<g->parameters.size();i++){double value=g->values[i].load(std::memory_order_relaxed);if(value!=p.previous[i]){p.previous[i]=value;p.changes.queues[i].value=value;p.changes.changed[p.changes.count++]=(int)i;}}p.inBus.silenceFlags=0;p.outBus.silenceFlags=0;p.context.projectTimeSamples=p.frames;p.context.projectTimeMusic=p.frames/48000.*2;ProcessData data;data.processMode=kRealtime;data.symbolicSampleSize=kSample32;data.numSamples=n;data.numInputs=data.numOutputs=1;data.inputs=&p.inBus;data.outputs=&p.outBus;data.inputParameterChanges=&p.changes;data.processContext=&p.context;auto result=p.processor->process(data);p.frames+=n;if(result!=kResultOk)return 0;for(int i=0;i<n;i++){float v=p.output[0][i];if(p.outputChannels==2)v=.5f*(v+p.output[1][i]);output[i]=std::isfinite(v)?v:0;}return 1;}
EXPORT int ql_parameters(Group* g,char* out,int cap){try{auto text=onUI([&]{std::ostringstream r;r<<"{\"Latency\":"<<g->voices[0]->processor->getLatencySamples()<<",\"NeedsRestart\":"<<(g->needsRestart?"true":"false")<<",\"Parameters\":[";for(size_t i=0;i<g->parameters.size();i++){auto& p=g->parameters[i];if(i)r<<",";String128 display{};double value=g->values[i].load();auto c=g->voices[0]->controller;if(c)c->getParamStringByValue(p.id,value,display);r<<"{\"ID\":"<<p.id<<",\"Name\":"<<quote(utf8(p.title))<<",\"Value\":"<<value<<",\"Display\":"<<quote(utf8(display))<<",\"Steps\":"<<p.stepCount<<",\"ReadOnly\":"<<((p.flags&ParameterInfo::kIsReadOnly)?"true":"false")<<",\"Hidden\":"<<((p.flags&ParameterInfo::kIsHidden)?"true":"false")<<"}";}r<<"]}";return r.str();});if((int)text.size()>=cap)throw std::runtime_error("Parameter list too large");copyText(out,cap,text);return 1;}catch(const std::exception& e){copyText(out,cap,e.what());return 0;}}
EXPORT int ql_set_parameter(Group* g,uint32 id,const double* value){try{return onUI([&]{if(!value||!std::isfinite(*value)||*value<0||*value>1)return 0;auto c=g->voices[0]->controller;if(!c)return 0;for(auto& p:g->parameters)if(p.id==id&&!(p.flags&ParameterInfo::kIsReadOnly)){if(c->setParamNormalized(id,*value)!=kResultOk)return 0;g->update(id,*value);return 1;}return 0;});}catch(...){return 0;}}
EXPORT int ql_editor(Group* g,char* error,int cap){try{return onUI([&]{if(g->frame.window){ShowWindow(g->frame.window,SW_RESTORE);SetForegroundWindow(g->frame.window);return 1;}auto c=g->voices[0]->controller;if(!c)throw std::runtime_error("This plugin has no editor; use Parameters instead");g->view=c->createView(ViewType::kEditor);if(!g->view)throw std::runtime_error("This plugin has no editor; use Parameters instead");if(g->view->isPlatformTypeSupported(kPlatformTypeHWND)!=kResultOk){release(g->view);throw std::runtime_error("Plugin editor does not support Windows");}WNDCLASSW wc{};wc.lpfnWndProc=editorProc;wc.hInstance=GetModuleHandleW(nullptr);wc.lpszClassName=L"QuietLabVST3Editor";wc.hCursor=LoadCursor(nullptr,IDC_ARROW);RegisterClassW(&wc);ViewRect size{0,0,800,600};g->view->getSize(&size);DWORD style=WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX;RECT r{0,0,size.getWidth(),size.getHeight()};AdjustWindowRect(&r,style,FALSE);g->frame.window=CreateWindowExW(0,wc.lpszClassName,L"Quiet Lab — VST3 Plugin",style,CW_USEDEFAULT,CW_USEDEFAULT,r.right-r.left,r.bottom-r.top,nullptr,nullptr,wc.hInstance,g);if(!g->frame.window){release(g->view);throw std::runtime_error("Cannot create plugin window");}g->view->setFrame(&g->frame);if(g->view->attached(g->frame.window,kPlatformTypeHWND)!=kResultOk){g->closeEditor();throw std::runtime_error("Plugin editor could not attach");}g->view->onSize(&size);ShowWindow(g->frame.window,SW_SHOW);return 1;});}catch(const std::exception& e){copyText(error,cap,e.what());return 0;}}
EXPORT int ql_state(Group* g,unsigned char* output,int cap){try{return onUI([&]{if(!output){Stream component,controller;if(g->voices[0]->component->getState(&component)!=kResultOk)throw std::runtime_error("Plugin state could not be saved");auto c=g->voices[0]->controller;if(c)c->getState(&controller);uint32 size=(uint32)component.bytes.size();g->cachedState.resize(4+size+controller.bytes.size());memcpy(g->cachedState.data(),&size,4);memcpy(g->cachedState.data()+4,component.bytes.data(),size);if(!controller.bytes.empty())memcpy(g->cachedState.data()+4+size,controller.bytes.data(),controller.bytes.size());}if(output){if(cap<(int)g->cachedState.size())return -1;memcpy(output,g->cachedState.data(),g->cachedState.size());}return (int)g->cachedState.size();});}catch(...){return -1;}}
EXPORT void ql_close(Group* g){if(g)onUI([&]{delete g;return 0;});}

