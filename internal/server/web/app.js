'use strict';
const $ = (id) => document.getElementById(id);
let token = ''; // Never in cookies, sessionStorage, localStorage or URLs.
let lastCase = null;
function node(tag,text,className){ const e=document.createElement(tag); if(text!==undefined)e.textContent=text; if(className)e.className=className; return e; }
function headers(extra={}) { return {...extra, ...(token?{Authorization:'Bearer '+token}:{})}; }
async function getJSON(path){ const r=await fetch(path,{headers:headers({Accept:'application/json'}),cache:'no-store'});if(!r.ok)throw new Error('HTTP '+r.status+' loading '+path);return r.json(); }
async function init(){
  try{
    const [info,claims]=await Promise.all([getJSON('/api/info'),getJSON('/api/claims?limit=40')]);
    $('dataset').textContent=`${info.dataset} · ${info.documents} documents`;
    $('use-model').disabled=!info.llm_available;
    $('use-jev').disabled=!info.jev_available;
    $('use-agents').disabled=!info.multiagent_available;
    $('use-agents').checked=Boolean(info.multiagent_available);
    $('sample').replaceChildren(node('option','Choose an example'));
    $('sample').firstChild.value='';
    for (const c of claims){const opt=node('option',c.claim);opt.value=c.claim;$('sample').append(opt);}
    $('auth-status').textContent='Connected';
    $('error').classList.add('hidden');
  }catch(e){$('dataset').textContent='Dataset unavailable';$('auth-status').textContent='Authentication or service unavailable';$('error').textContent=e.message;$('error').classList.remove('hidden');}
}
$('connect').addEventListener('click',()=>{ token=$('api-token').value.trim();$('api-token').value='';init(); });
$('sample').addEventListener('change',()=>{if($('sample').value)$('claim').value=$('sample').value;});
$('use-model').addEventListener('change',()=>{if($('use-model').checked){$('use-jev').checked=false;$('use-agents').checked=false;}});
$('use-jev').addEventListener('change',()=>{if($('use-jev').checked){$('use-model').checked=false;$('use-agents').checked=false;}});
$('use-agents').addEventListener('change',()=>{if($('use-agents').checked){$('use-model').checked=false;$('use-jev').checked=false;}});
$('investigate-form').addEventListener('submit',async(e)=>{
  e.preventDefault(); $('error').classList.add('hidden'); $('submit').disabled=true; $('submit').textContent='Investigating…';
  try{
    const response=await fetch('/api/investigate',{method:'POST',headers:headers({'Content-Type':'application/json'}),body:JSON.stringify({claim:$('claim').value,top_k:Number($('topk').value),use_model:$('use-model').checked,use_jev:$('use-jev').checked,use_agents:$('use-agents').checked})});
    const report=await response.json();if(!response.ok)throw new Error(report.error||'Investigation failed');
    $('results').classList.remove('hidden');$('verdict').textContent=report.verdict.replaceAll('_',' ');$('notice').textContent=report.notice;
    $('timing').textContent=`${report.took_ms} ms · ${report.dataset}`;
    $('interpretation').textContent=report.jev_report?report.jev_report.narrative.summary:(report.assessment?report.assessment.explanation:'');
    const d=report.jev_report?.decision;
    $('agent-details').textContent=report.agent_trace?`Peer agents: ${report.agent_trace.stop_reason}; ${report.agent_trace.model_calls} LLM calls; ${report.agent_trace.decision_calls} JEV decisions; ${report.agent_trace.events.length} audited actions`:'';
    $('jev-details').textContent=d?`JEV choice ${d.jev_choice}; policy ${d.verdict}; model confidence ${d.confidence.toFixed(3)} (not verified correctness); ${d.policy_reason}`:'';
    $('count').textContent=`${report.evidence.length} retrieved passages`;
    const holder=$('passages');holder.replaceChildren();
    for (const item of report.evidence){
      const article=node('article',undefined,'passage');const header=node('div',undefined,'passage-header');
      header.append(node('div',item.title,'passage-title'),node('span',item.score.toFixed(3),'muted'));article.append(header);
      article.append(node('p',`${item.source_id} · sentence ${item.sentence_index}`,'passage-meta'));
      article.append(node('blockquote',item.quote));article.append(node('p',`SHA-256 ${item.sha256}`,'passage-footer'));holder.append(article);
    }
    lastCase=report.case_id?{id:report.case_id,version:report.case_version}:null;
    $('review-panel').classList.toggle('hidden',!lastCase);
    $('case-id').textContent=lastCase?.id||'';
    $('review-status').textContent=lastCase?'Pending human review':'';
    document.querySelectorAll('[data-review]').forEach(b=>b.disabled=!lastCase);
    $('results').scrollIntoView({behavior:'smooth',block:'start'});
  }catch(err){$('error').textContent=err.message;$('error').classList.remove('hidden');}
  finally{$('submit').disabled=false;$('submit').textContent='Find evidence ↗';}
});
document.querySelectorAll('[data-review]').forEach(btn=>btn.addEventListener('click',async()=>{
 if(!lastCase)return;
 const actor=$('reviewer').value.trim(),note=$('review-note').value.trim();
 if(!actor){$('error').textContent='Please provide the separate review token.';$('error').classList.remove('hidden');return;}
 btn.disabled=true;
 try{
  const r=await fetch('/api/cases/'+lastCase.id+'/review',{method:'POST',headers:headers({'Content-Type':'application/json','X-Review-Token':actor}),body:JSON.stringify({version:lastCase.version,status:btn.dataset.review,actor:'authenticated-reviewer',note})});
  const data=await r.json();if(!r.ok)throw new Error(data.error||'Review failed');
  lastCase.version=data.version;$('review-status').textContent='Reviewed: '+data.status;
  document.querySelectorAll('[data-review]').forEach(b=>b.disabled=true);
  $('error').classList.add('hidden');
 }catch(err){$('error').textContent=err.message;$('error').classList.remove('hidden');btn.disabled=false;}
}));
init();
