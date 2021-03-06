/*                                                                
                                                                
      @@@@@@@@   @@@@@@   @@@       @@@@@@@@  @@@@@@@@@@        
     @@@@@@@@@  @@@@@@@@  @@@       @@@@@@@@  @@@@@@@@@@@       
     !@@        @@!  @@@  @@!       @@!       @@! @@! @@!       
     !@!        !@!  @!@  !@!       !@!       !@! !@! !@!       
     !@! @!@!@  @!@  !@!  @!!       @!!!:!    @!! !!@ @!@       
     !!! !!@!!  !@!  !!!  !!!       !!!!!:    !@!   ! !@!       
     :!!   !!:  !!:  !!!  !!:       !!:       !!:     !!:       
     :!:   !::  :!:  !:!   :!:      :!:       :!:     :!:       
      ::: ::::  ::::: ::   :: ::::   :: ::::  :::     ::        
      :: :: :    : :  :   : :: : :  : :: ::    :      :         

    This script will run a golem when loaded, but only on
    a headless Chrome and only one per webstrate.

    You can can control this golem with the following 
    commands:

    To *reload* the golem, i.e. reset its state and run it
    again:

    $ curl https://{{ .BaseURL }}/golem/v1/reset/<webstrate-id>

    To *kill* the golem (the golem will respawn the next
    time the page is loaded):

    $ curl https://{{ .BaseURL }}/golem/v1/kill/<webstrate-id>
                                                                
*/

(function(){
  if (!/headless/i.exec(navigator.userAgent)) { 
    // You're human, probably.
    // Send request to spawn golem, multiple requests
    // for the same webstrate will be ignored
    var spawnRequest = new XMLHttpRequest();
    spawnRequest.onreadystatechange = function() {
      if (spawnRequest.readyState === XMLHttpRequest.DONE) {
        if (spawnRequest.status === 200) {
          console.log("Golem spawned for "+webstrate.webstrateId);
        } else {
          console.warn("Golem could not be spawned - "+spawnRequest.responseText);
        }
      }
    }; 
    spawnRequest.open('GET', 'https://{{ .BaseURL }}/golem/v1/spawn/'+webstrate.webstrateId, true);
    spawnRequest.send();
  } else {
    // You're a golem, most likely.
    // Bootstrap yourself. Rise! 
    window.setTimeout(function() {
      new Function(document.querySelector(".golem,#emet").textContent)();
    }, 5000);
  }
})();
